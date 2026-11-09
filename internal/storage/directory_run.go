package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/transfer"
	"mcl/internal/fileio"
)

func stageName(plan app.DirectoryPlan) string  { return ".mcl-dir-stage-" + plan.Token }
func backupName(plan app.DirectoryPlan) string { return ".mcl-dir-backup-" + plan.Token }
func planParts(plan app.DirectoryPlan) []string {
	if plan.Kind == "import" {
		return []string{"game"}
	}
	var parts []string
	for _, part := range payloadParts {
		if slices.Contains(plan.Tree.Directories, part) {
			parts = append(parts, part)
		}
	}
	return parts
}
func stampDirectory(ctx context.Context, root *os.Root, name string, plan app.DirectoryPlan) error {
	err := readDirectoryOwner(ctx, root, name, plan)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeDirectoryJSON(ctx, root, name, ownerFor(plan))
}
func (s *DirectoryTransferStore) stage(ctx context.Context, parent *os.Root, plan app.DirectoryPlan) (*os.Root, *os.Root, error) {
	name := stageName(plan)
	if err := parent.Mkdir(name, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, nil, err
	}
	stage, err := openRealDirectory(parent, name)
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*os.Root, *os.Root, error) { stage.Close(); return nil, nil, err }
	if err := readDirectoryOwner(ctx, stage, transfer.OwnerFile, plan); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fail(err)
		}
		f, e := stage.Open(".")
		if e != nil {
			return fail(e)
		}
		entries, e := f.ReadDir(1)
		f.Close()
		if e != nil && !errors.Is(e, io.EOF) {
			return fail(e)
		}
		if len(entries) != 0 {
			return fail(errors.New("未标记的暂存目录已有内容，未接管"))
		}
		if e := writeDirectoryJSON(ctx, stage, transfer.OwnerFile, ownerFor(plan)); e != nil {
			return fail(e)
		}
	}
	if err := stage.Mkdir("payload", 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fail(err)
	}
	payload, err := openRealDirectory(stage, "payload")
	if err != nil {
		return fail(err)
	}
	if err := stampDirectory(ctx, payload, transfer.OwnerFile, plan); err != nil {
		payload.Close()
		return fail(err)
	}
	return stage, payload, nil
}
func (s *DirectoryTransferStore) sourceForPlan(ctx context.Context, plan app.DirectoryPlan) (*os.Root, error) {
	if plan.Kind == "import" {
		_, fingerprint, root, err := s.source(ctx, plan.SourceID, plan.CandidateID, plan.DataPath)
		if err != nil {
			return nil, err
		}
		if fingerprint != plan.SourceFingerprint {
			root.Close()
			return nil, errors.New("来源版本元数据链发生变化，请撤销后重新预览")
		}
		return root, nil
	}
	value, err := NewInstanceStore(filepath.Join(s.root, "instances")).Get(ctx, plan.After.ID)
	if err != nil {
		return nil, err
	}
	if instanceFingerprint(value) != instanceFingerprint(plan.Before) {
		return nil, errors.New("实例信息已改变，停止复制")
	}
	root, err := s.payload(ctx, plan.Before.GameDirectoryID, plan.Before.ID)
	if err != nil {
		return nil, err
	}
	if err := checkPayloadSource(ctx, root, plan.Before.ID, plan.Before.GameDirectoryID != ""); err != nil {
		root.Close()
		return nil, err
	}
	return root, nil
}
func targetData(ctx context.Context, parent *os.Root, plan app.DirectoryPlan) (*os.Root, error) {
	payload, err := parent.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	if plan.After.GameDirectoryID != "" {
		payload.Close()
		payload, err = openRealDirectory(parent, plan.After.ID)
		if err != nil {
			return nil, err
		}
		if err := readDirectoryOwner(ctx, payload, transfer.OwnerFile, plan); err != nil {
			payload.Close()
			return nil, err
		}
	}
	if plan.After.GameDirectoryID == "" {
		for _, part := range planParts(plan) {
			node, e := openRealDirectory(payload, part)
			if e != nil {
				payload.Close()
				return nil, e
			}
			e = readDirectoryOwner(ctx, node, transfer.OwnerFile, plan)
			node.Close()
			if e != nil {
				payload.Close()
				return nil, e
			}
		}
	}
	if plan.Kind == "move" {
		return payload, nil
	}
	defer payload.Close()
	return openRealDirectory(payload, "game")
}
func (s *DirectoryTransferStore) Run(ctx context.Context, plan app.DirectoryPlan, report func(transfer.Progress)) (instance.Instance, error) {
	state, err := s.state(ctx, plan.After.ID)
	if err != nil {
		return instance.Instance{}, err
	}
	if state.Plan.Fingerprint() != plan.Fingerprint() {
		return instance.Instance{}, errors.New("目录操作计划已变化")
	}
	if state.Phase == "aborting" {
		return instance.Instance{}, errors.New("此任务正在撤销，请继续撤销")
	}
	verifiedTarget := false
	parent, err := s.parent(ctx, plan.After.GameDirectoryID, plan.After.ID)
	if err != nil {
		return instance.Instance{}, err
	}
	defer parent.Close()
	// Atomic metadata replacement may have succeeded just before interruption.
	if plan.Kind == "move" && state.Phase == "publishing" {
		value, e := NewInstanceStore(filepath.Join(s.root, "instances")).Get(ctx, plan.After.ID)
		if e != nil {
			return instance.Instance{}, e
		}
		if instanceFingerprint(value) == instanceFingerprint(plan.After) {
			state.Phase = "committed"
			if err := s.writeState(ctx, state); err != nil {
				return value, err
			}
		}
	}
	if state.Phase == "publishing" {
		restage, e := s.needsRestaging(ctx, parent, plan)
		if e != nil {
			return instance.Instance{}, e
		}
		if restage {
			state.Phase = "copying"
			if err := s.writeState(ctx, state); err != nil {
				return instance.Instance{}, err
			}
		}
	}
	if state.Phase == "copying" {
		source, err := s.sourceForPlan(ctx, plan)
		if err != nil {
			return instance.Instance{}, err
		}
		if err := transfer.Verify(ctx, source, plan.Tree, plan.Kind == "import"); err != nil {
			source.Close()
			return instance.Instance{}, err
		}
		stage, payload, err := s.stage(ctx, parent, plan)
		if err != nil {
			source.Close()
			return instance.Instance{}, err
		}
		data := payload
		if plan.Kind == "import" {
			if err = payload.Mkdir("game", 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				source.Close()
				payload.Close()
				stage.Close()
				return instance.Instance{}, err
			}
			data, err = openRealDirectory(payload, "game")
			if err != nil {
				source.Close()
				payload.Close()
				stage.Close()
				return instance.Instance{}, err
			}
		}
		err = cleanCopyTemps(ctx, data, plan.Tree)
		if err == nil {
			err = transfer.Copy(ctx, source, data, plan.Tree, report)
		}
		if err == nil {
			err = transfer.VerifyCopied(ctx, data, plan.Tree)
		}
		if err == nil {
			err = transfer.Verify(ctx, source, plan.Tree, plan.Kind == "import")
		}
		if data != payload {
			data.Close()
		}
		source.Close()
		if err == nil {
			for _, part := range planParts(plan) {
				node, e := openRealDirectory(payload, part)
				if e != nil {
					err = e
					break
				}
				e = stampDirectory(ctx, node, transfer.OwnerFile, plan)
				node.Close()
				if e != nil {
					err = e
					break
				}
			}
		}
		if err == nil {
			err = checkStageTop(payload, planParts(plan))
		}
		if err == nil && plan.Kind == "import" {
			game, e := openRealDirectory(payload, "game")
			if e != nil {
				err = e
			} else {
				err = checkStageTop(game, plan.Tree.Roots)
				game.Close()
			}
		}
		payload.Close()
		stage.Close()
		if err != nil {
			return instance.Instance{}, err
		}
		state.Phase = "publishing"
		if err := s.writeState(ctx, state); err != nil {
			return instance.Instance{}, err
		}
	}
	if state.Phase == "publishing" {
		source, err := s.sourceForPlan(ctx, plan)
		if err != nil {
			return instance.Instance{}, err
		}
		err = transfer.Verify(ctx, source, plan.Tree, plan.Kind == "import")
		source.Close()
		if err != nil {
			return instance.Instance{}, err
		}
		if err := s.publish(ctx, parent, plan); err != nil {
			return instance.Instance{}, err
		}
		actual, err := targetData(ctx, parent, plan)
		if err != nil {
			return instance.Instance{}, err
		}
		err = transfer.VerifyCopied(ctx, actual, plan.Tree)
		actual.Close()
		if err != nil {
			return instance.Instance{}, err
		}
		// Reopen foreign metadata as well as game data before the commit point.
		source, err = s.sourceForPlan(ctx, plan)
		if err != nil {
			return instance.Instance{}, err
		}
		err = transfer.Verify(ctx, source, plan.Tree, plan.Kind == "import")
		source.Close()
		if err != nil {
			return instance.Instance{}, err
		}
		verifiedTarget = true
		if plan.Kind == "move" {
			if err := s.commitLocation(ctx, plan); err != nil {
				return instance.Instance{}, err
			}
		}
		state.Phase = "committed"
		if err := s.writeState(context.WithoutCancel(ctx), state); err != nil {
			return plan.After, err
		}
	}
	if !verifiedTarget && (state.Phase == "committed" || state.Phase == "backing-up" || state.Phase == "complete") {
		data, e := targetData(ctx, parent, plan)
		if e == nil {
			e = transfer.VerifyCopied(ctx, data, plan.Tree)
			data.Close()
		}
		if e != nil {
			if err := s.restoreCommitted(ctx, parent, plan, report); err != nil {
				return plan.After, err
			}
		}
		data, e = targetData(ctx, parent, plan)
		if e != nil {
			return plan.After, e
		}
		e = transfer.VerifyCopied(ctx, data, plan.Tree)
		data.Close()
		if e != nil {
			return plan.After, e
		}
		verifiedTarget = true
		if plan.Kind == "move" {
			if err := s.ensureCommittedLocation(ctx, plan); err != nil {
				return plan.After, err
			}
		}
	}
	if state.Phase == "committed" || state.Phase == "backing-up" {
		if plan.Kind == "move" {
			if report != nil {
				report(transfer.Progress{Stage: "保留旧位置备份", Files: len(plan.Tree.Files), TotalFiles: len(plan.Tree.Files), Bytes: plan.Tree.Bytes, TotalBytes: plan.Tree.Bytes})
			}
			state.Phase = "backing-up"
			if err := s.writeState(ctx, state); err != nil {
				return plan.After, err
			}
			if err := s.backup(ctx, &state); err != nil {
				return plan.After, err
			}
		}
		state.Phase = "complete"
		if err := s.writeState(ctx, state); err != nil {
			return plan.After, err
		}
	}
	if state.Phase == "complete" {
		if plan.Kind == "import" {
			if err := s.recordImport(ctx, plan); err != nil {
				return plan.After, err
			}
		}
		if err := s.removeStage(ctx, parent, plan); err != nil {
			return plan.After, err
		}
		if plan.Kind == "move" {
			if err := s.saveHistory(ctx, state); err != nil {
				return plan.After, err
			}
		}
		central, err := s.central(plan.After.ID)
		if err != nil {
			return plan.After, err
		}
		defer central.Close()
		if err := central.Remove(directoryPending); err != nil {
			return plan.After, err
		}
		if err := fileio.SyncDirectory(central); err != nil {
			return plan.After, err
		}
	}
	return plan.After, nil
}
func (s *DirectoryTransferStore) publish(ctx context.Context, parent *os.Root, plan app.DirectoryPlan) error {
	if plan.After.GameDirectoryID != "" {
		if final, err := openRealDirectory(parent, plan.After.ID); err == nil {
			defer final.Close()
			return readDirectoryOwner(ctx, final, transfer.OwnerFile, plan)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		stage, err := openRealDirectory(parent, stageName(plan))
		if err != nil {
			return err
		}
		defer stage.Close()
		if err := readDirectoryOwner(ctx, stage, transfer.OwnerFile, plan); err != nil {
			return err
		}
		return directoryMove(stage, "payload", parent, plan.After.ID)
	}
	var stage, payload *os.Root
	defer func() {
		if payload != nil {
			payload.Close()
		}
		if stage != nil {
			stage.Close()
		}
	}()
	for _, part := range planParts(plan) {
		if err := ctx.Err(); err != nil {
			return err
		}
		final, err := openRealDirectory(parent, part)
		if err == nil {
			err = readDirectoryOwner(ctx, final, transfer.OwnerFile, plan)
			final.Close()
			if err != nil {
				return err
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if stage == nil {
			stage, err = openRealDirectory(parent, stageName(plan))
			if err != nil {
				return err
			}
			if err := readDirectoryOwner(ctx, stage, transfer.OwnerFile, plan); err != nil {
				return err
			}
			payload, err = openRealDirectory(stage, "payload")
			if err != nil {
				return err
			}
		}
		if err := directoryMove(payload, part, parent, part); err != nil {
			return err
		}
	}
	return nil
}
func (s *DirectoryTransferStore) commitLocation(ctx context.Context, plan app.DirectoryPlan) error {
	root, err := s.central(plan.After.ID)
	if err != nil {
		return err
	}
	defer root.Close()
	var document instanceDocument
	if err := readUpdateJSON(ctx, root, "instance.json", 512<<10, &document); err != nil {
		return err
	}
	if instanceFingerprint(document.Instance) != instanceFingerprint(plan.Before) {
		return errors.New("实例配置在位置提交前发生变化")
	}
	document.Instance = plan.After
	document.Version = max(document.Version, instanceDocumentVersion(plan.After))
	return writeDirectoryJSON(ctx, root, "instance.json", document)
}
func (s *DirectoryTransferStore) removeStage(ctx context.Context, parent *os.Root, plan app.DirectoryPlan) error {
	root, err := openRealDirectory(parent, stageName(plan))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := readDirectoryOwner(ctx, root, transfer.OwnerFile, plan); err != nil {
		root.Close()
		return err
	}
	// Recovery may have recopied already published nodes. Reclaim only exact
	// duplicate staged data, after the committed destination has verified.
	payload, err := openRealDirectory(root, "payload")
	if err == nil {
		err = verifyStagedRemainder(ctx, payload, plan)
		payload.Close()
		if err != nil {
			root.Close()
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		root.Close()
		return err
	}
	f, e := root.Open(".")
	if e != nil {
		root.Close()
		return e
	}
	entries, e := f.ReadDir(3)
	f.Close()
	root.Close()
	if e != nil && !errors.Is(e, io.EOF) {
		return e
	}
	for _, entry := range entries {
		if entry.Name() != transfer.OwnerFile && entry.Name() != "payload" {
			return errors.New("暂存容器出现额外文件，已保留")
		}
	}
	return parent.RemoveAll(stageName(plan))
}
func cleanCopyTemps(ctx context.Context, root *os.Root, tree transfer.Tree) error {
	planned := map[string]bool{}
	for _, file := range tree.Files {
		planned[file.Path] = true
	}
	directories := append([]string{"."}, tree.Directories...)
	for _, name := range directories {
		if err := ctx.Err(); err != nil {
			return err
		}
		dir, err := transfer.OpenDirectory(root, name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		f, err := dir.Open(".")
		if err != nil {
			dir.Close()
			return err
		}
		entries, err := f.ReadDir(transfer.MaxEntries + 1)
		f.Close()
		if err != nil && !errors.Is(err, io.EOF) {
			dir.Close()
			return err
		}
		if len(entries) > transfer.MaxEntries {
			dir.Close()
			return errors.New("暂存目录项过多")
		}
		for _, entry := range entries {
			relative := entry.Name()
			if name != "." {
				relative = name + "/" + entry.Name()
			}
			if strings.HasPrefix(entry.Name(), ".mcl-copy-") && !planned[relative] {
				info, e := dir.Lstat(entry.Name())
				if e != nil {
					dir.Close()
					return e
				}
				if !info.Mode().IsRegular() {
					dir.Close()
					return errors.New("暂存分片不是普通文件")
				}
				if e := dir.Remove(entry.Name()); e != nil {
					dir.Close()
					return e
				}
			}
		}
		dir.Close()
	}
	return nil
}
