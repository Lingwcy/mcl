package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/transfer"
)

func partTree(tree transfer.Tree, part string) transfer.Tree {
	result := transfer.Tree{Roots: []string{part}}
	for _, directory := range tree.Directories {
		if directory == part || strings.HasPrefix(directory, part+"/") {
			result.Directories = append(result.Directories, directory)
		}
	}
	for _, file := range tree.Files {
		if strings.HasPrefix(file.Path, part+"/") {
			result.Files = append(result.Files, file)
			result.Bytes += file.Size
		}
	}
	return result
}
func emptyOwnedDirectory(ctx context.Context, parent *os.Root, name string, plan app.DirectoryPlan) (*os.Root, error) {
	if err := parent.Mkdir(name, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	root, err := openRealDirectory(parent, name)
	if err != nil {
		return nil, err
	}
	if err := readDirectoryOwner(ctx, root, transfer.OwnerFile, plan); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			root.Close()
			return nil, err
		}
		f, e := root.Open(".")
		if e != nil {
			root.Close()
			return nil, e
		}
		entries, e := f.ReadDir(1)
		f.Close()
		if e != nil && !errors.Is(e, io.EOF) {
			root.Close()
			return nil, e
		}
		if len(entries) > 0 {
			root.Close()
			return nil, errors.New("目标缺少所属证据且已有内容，未接管")
		}
		if err := stampDirectory(ctx, root, transfer.OwnerFile, plan); err != nil {
			root.Close()
			return nil, err
		}
	}
	return root, nil
}

// Interrupted directory-entry publication can leave a valid fixed plan but no
// private staging on its volume. Recopy from the unchanged source, never infer
// success from a missing stage or replace an occupied destination.
func (s *DirectoryTransferStore) needsRestaging(ctx context.Context, parent *os.Root, plan app.DirectoryPlan) (bool, error) {
	names := planParts(plan)
	if plan.After.GameDirectoryID != "" {
		names = []string{plan.After.ID}
	}
	missing := false
	for _, name := range names {
		node, err := openRealDirectory(parent, name)
		if errors.Is(err, os.ErrNotExist) {
			missing = true
			continue
		}
		if err != nil {
			return false, err
		}
		err = readDirectoryOwner(ctx, node, transfer.OwnerFile, plan)
		node.Close()
		if err != nil {
			return false, err
		}
	}
	if !missing {
		return false, nil
	}
	stage, err := openRealDirectory(parent, stageName(plan))
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer stage.Close()
	if err := readDirectoryOwner(ctx, stage, transfer.OwnerFile, plan); err != nil {
		return false, err
	}
	payload, err := openRealDirectory(stage, "payload")
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	defer payload.Close()
	for _, part := range planParts(plan) {
		if plan.After.GameDirectoryID == "" {
			if final, err := openRealDirectory(parent, part); err == nil {
				final.Close()
				continue
			} else if !errors.Is(err, os.ErrNotExist) {
				return false, err
			}
		}
		if _, err := payload.Lstat(part); errors.Is(err, os.ErrNotExist) {
			return true, nil
		} else if err != nil {
			return false, err
		}
	}
	return false, nil
}

// Only fills missing files at the already committed, owned destination. Changed
// or unowned target entries are preserved and produce an explicit conflict.
// This is explicit recovery, not routing games to a hidden built-in fallback.
func (s *DirectoryTransferStore) restoreCommitted(ctx context.Context, parent *os.Root, plan app.DirectoryPlan, report func(transfer.Progress)) error {
	target, err := parent.OpenRoot(".")
	if err != nil {
		return err
	}
	if plan.After.GameDirectoryID != "" {
		target.Close()
		target, err = emptyOwnedDirectory(ctx, parent, plan.After.ID, plan)
		if err != nil {
			return err
		}
	}
	defer target.Close()
	if plan.Kind == "import" {
		source, err := s.sourceForPlan(ctx, plan)
		if err != nil {
			return err
		}
		defer source.Close()
		game, err := emptyOwnedDirectory(ctx, target, "game", plan)
		if err != nil {
			return err
		}
		defer game.Close()
		return transfer.Copy(ctx, source, game, plan.Tree, report)
	}
	oldParent, err := s.parent(ctx, plan.Before.GameDirectoryID, plan.Before.ID)
	if err != nil {
		return err
	}
	defer oldParent.Close()
	backup, err := openRealDirectory(oldParent, backupName(plan))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if backup != nil {
		defer backup.Close()
		if err := readDirectoryOwner(ctx, backup, directoryBackupOwner, plan); err != nil {
			return err
		}
	}
	original, sourceErr := s.payload(ctx, plan.Before.GameDirectoryID, plan.Before.ID)
	if sourceErr != nil && !errors.Is(sourceErr, os.ErrNotExist) {
		return sourceErr
	}
	if original != nil {
		defer original.Close()
	}
	for _, part := range planParts(plan) {
		source := backup
		if source != nil {
			if _, err := source.Lstat(part); errors.Is(err, os.ErrNotExist) {
				source = nil
			} else if err != nil {
				return err
			}
		}
		if source == nil {
			source = original
		}
		if source == nil {
			return errors.New("已提交位置不完整，原载荷与保留副本均不可用，未创建空成功结果")
		}
		node, err := emptyOwnedDirectory(ctx, target, part, plan)
		if err != nil {
			return err
		}
		node.Close()
		if err := transfer.Copy(ctx, source, target, partTree(plan.Tree, part), report); err != nil {
			return err
		}
	}
	return nil
}

func (s *DirectoryTransferStore) ensureCommittedLocation(ctx context.Context, plan app.DirectoryPlan) error {
	current, err := NewInstanceStore(filepath.Join(s.root, "instances")).Get(ctx, plan.After.ID)
	if err != nil {
		return err
	}
	if instanceFingerprint(current) == instanceFingerprint(plan.After) {
		return nil
	}
	if instanceFingerprint(current) == instanceFingerprint(plan.Before) {
		return s.commitLocation(ctx, plan)
	}
	return errors.New("提交恢复时实例配置发生变化，未覆盖")
}
func verifyStagedRemainder(ctx context.Context, payload *os.Root, plan app.DirectoryPlan) error {
	if err := checkStageTop(payload, planParts(plan)); err != nil {
		return err
	}
	for _, part := range planParts(plan) {
		node, err := openRealDirectory(payload, part)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := readDirectoryOwner(ctx, node, transfer.OwnerFile, plan); err != nil {
			node.Close()
			return err
		}
		if plan.Kind == "import" {
			err = checkStageTop(node, plan.Tree.Roots)
			if err == nil {
				err = transfer.VerifyCopied(ctx, node, plan.Tree)
			}
		} else {
			err = transfer.VerifyCopied(ctx, payload, partTree(plan.Tree, part))
		}
		node.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
