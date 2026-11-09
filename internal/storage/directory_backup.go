package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/transfer"
	"mcl/internal/fileio"
)

func (s *DirectoryTransferStore) backup(ctx context.Context, state *directoryState) error {
	p := state.Plan
	parent, err := s.parent(ctx, p.Before.GameDirectoryID, p.Before.ID)
	if err != nil {
		return err
	}
	defer parent.Close()
	name := backupName(p)
	backup, err := openRealDirectory(parent, name)
	if errors.Is(err, os.ErrNotExist) {
		if p.Before.GameDirectoryID != "" {
			old, err := openRealDirectory(parent, p.Before.ID)
			if err != nil {
				return err
			}
			if err := stampDirectory(ctx, old, directoryBackupOwner, p); err != nil {
				old.Close()
				return err
			}
			old.Close()
			if err := directoryMove(parent, p.Before.ID, parent, name); err != nil {
				return err
			}
		} else {
			if err := parent.Mkdir(name, 0o700); err != nil {
				return err
			}
			created, err := openRealDirectory(parent, name)
			if err != nil {
				return err
			}
			err = writeDirectoryJSON(ctx, created, directoryBackupOwner, ownerFor(p))
			created.Close()
			if err != nil {
				return err
			}
		}
		backup, err = openRealDirectory(parent, name)
	}
	if err != nil {
		return err
	}
	defer backup.Close()
	if err := readDirectoryOwner(ctx, backup, directoryBackupOwner, p); err != nil {
		// A crash can occur after an empty built-in backup directory was created.
		if !errors.Is(err, os.ErrNotExist) || p.Before.GameDirectoryID != "" {
			return err
		}
		f, e := backup.Open(".")
		if e != nil {
			return e
		}
		entries, e := f.ReadDir(1)
		f.Close()
		if e != nil && !errors.Is(e, io.EOF) {
			return e
		}
		if len(entries) != 0 {
			return errors.New("未标记备份已有内容，未接管")
		}
		if err := writeDirectoryJSON(ctx, backup, directoryBackupOwner, ownerFor(p)); err != nil {
			return err
		}
	}
	if p.Before.GameDirectoryID == "" {
		for _, part := range payloadParts {
			original, srcErr := parent.Lstat(part)
			_, dstErr := backup.Lstat(part)
			if srcErr == nil {
				if dstErr == nil {
					return errors.New("源位置与备份同时有同名目录，已保留两者")
				}
				if !errors.Is(dstErr, os.ErrNotExist) {
					return dstErr
				}
				if !original.IsDir() || original.Mode()&os.ModeSymlink != 0 {
					return errors.New("旧载荷身份发生变化")
				}
				if err := directoryMove(parent, part, backup, part); err != nil {
					return err
				}
			} else if !errors.Is(srcErr, os.ErrNotExist) {
				return srcErr
			} else if errors.Is(dstErr, os.ErrNotExist) && slices.Contains(p.Tree.Directories, part) {
				return errors.New("原载荷与备份同时缺失，不能完成迁移")
			}
		}
	}
	// The backup contains the actual old copy, including edits made by another
	// program after publication. Such edits are not silently erased or claimed
	// to have been copied to the new location.
	roots, err := backupRoots(backup)
	if err != nil {
		return err
	}
	tree, err := transfer.Scan(ctx, backup, roots, false, nil)
	if err != nil {
		return err
	}
	payload, err := transfer.Scan(ctx, backup, payloadParts, false, nil)
	if err != nil {
		return err
	}
	state.BackupChanged = payload.Fingerprint() != p.Tree.Fingerprint()
	state.BackupTree = &tree
	return s.writeState(ctx, *state)
}
func backupRoots(root *os.Root) ([]string, error) {
	f, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(transfer.MaxEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > transfer.MaxEntries {
		return nil, errors.New("备份顶层数量越界")
	}
	var names []string
	for _, entry := range entries {
		if entry.Name() != directoryBackupOwner && entry.Name() != transfer.OwnerFile {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}
func (s *DirectoryTransferStore) saveHistory(ctx context.Context, state directoryState) error {
	root, err := s.central(state.Plan.After.ID)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Mkdir("directory-history", 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	history, err := openRealDirectory(root, "directory-history")
	if err != nil {
		return err
	}
	defer history.Close()
	var existing directoryState
	err = readUpdateJSON(ctx, history, state.Plan.Token+".json", 32<<20, &existing)
	if err == nil && existing.Plan.Fingerprint() != state.Plan.Fingerprint() {
		return errors.New("备份历史记录冲突，未覆盖")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeDirectoryJSON(ctx, history, state.Plan.Token+".json", state)
}
func (s *DirectoryTransferStore) CleanBackup(ctx context.Context, id, token string, trash func(string) error) error {
	if !instance.ValidID(token) || trash == nil {
		return errors.New("备份标识或系统废纸篓服务无效")
	}
	if _, err := s.Pending(ctx, id); err == nil {
		return errors.New("请先完成未结束的目录操作")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	root, err := s.central(id)
	if err != nil {
		return err
	}
	defer root.Close()
	history, err := openRealDirectory(root, "directory-history")
	if err != nil {
		return err
	}
	defer history.Close()
	var state directoryState
	if err := readUpdateJSON(ctx, history, token+".json", 32<<20, &state); err != nil {
		return err
	}
	if err := validateDirectoryState(id, state); err != nil {
		return err
	}
	if state.Plan.Token != token || state.Phase != "complete" || state.Plan.Kind != "move" || state.BackupTree == nil {
		return errors.New("备份记录不完整，未清理")
	}
	if state.BackupGone {
		return nil
	}
	if err := validateTransferTree(*state.BackupTree, false); err != nil {
		return err
	}
	// A current owned game must still exist. Deleting a historical backup must
	// never leave the instance pointing at an unavailable or redirected volume.
	current, err := NewAssetStore(filepath.Join(s.root, "instances")).OpenGame(ctx, id)
	if err != nil {
		return err
	}
	current.Close()
	parent, err := s.parent(ctx, state.Plan.Before.GameDirectoryID, id)
	if err != nil {
		return err
	}
	defer parent.Close()
	backup, err := openRealDirectory(parent, backupName(state.Plan))
	if errors.Is(err, os.ErrNotExist) {
		state.BackupGone = true
		return finishBackupCleanup(ctx, history, state)
	}
	if err != nil {
		return err
	}
	if err := readDirectoryOwner(ctx, backup, directoryBackupOwner, state.Plan); err != nil {
		backup.Close()
		return err
	}
	roots, err := backupRoots(backup)
	if err == nil && !slices.Equal(roots, state.BackupTree.Roots) {
		err = errors.New("备份出现额外内容，请先核对")
	}
	if err == nil {
		err = transfer.VerifyCopied(ctx, backup, *state.BackupTree)
	}
	name := backup.Name()
	backup.Close()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := trash(name); err != nil {
		return err
	}
	state.BackupGone = true
	return finishBackupCleanup(context.WithoutCancel(ctx), history, state)
}
func (s *DirectoryTransferStore) Abort(ctx context.Context, id, token string, trash func(string) error) error {
	if trash == nil {
		return errors.New("系统废纸篓服务未配置")
	}
	state, err := s.state(ctx, id)
	if err != nil {
		return err
	}
	p := state.Plan
	if p.Token != token {
		return errors.New("目录任务已改变，请重新读取")
	}
	if state.Phase == "committed" || state.Phase == "backing-up" || state.Phase == "complete" {
		return errors.New("新位置已提交，请完成恢复；回迁需要另起迁移")
	}
	if p.Kind == "move" {
		current, err := NewInstanceStore(filepath.Join(s.root, "instances")).Get(ctx, id)
		if err != nil {
			return err
		}
		if instanceFingerprint(current) != instanceFingerprint(p.Before) {
			return errors.New("位置已提交或实例变化，不能撤销，请继续恢复")
		}
	}
	state.Phase = "aborting"
	if err := s.writeState(ctx, state); err != nil {
		return err
	}
	parent, err := s.parent(ctx, p.After.GameDirectoryID, id)
	if err != nil {
		return err
	}
	names := planParts(p)
	if p.After.GameDirectoryID != "" {
		names = []string{id}
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			parent.Close()
			return err
		}
		node, err := openRealDirectory(parent, name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			parent.Close()
			return err
		}
		if err := readDirectoryOwner(ctx, node, transfer.OwnerFile, p); err != nil {
			node.Close()
			// A conflicting entry without our exact stamp is not this transaction's
			// property. Undo our private work, but leave that entry untouched.
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, errDirectoryOwner) {
				if p.Kind == "import" && p.After.GameDirectoryID == "" {
					parent.Close()
					return errors.New("新实例内出现未属于本次复制的载荷，未回收该实例目录")
				}
				continue
			}
			parent.Close()
			return err
		}
		target := node.Name()
		node.Close()
		if err := trash(target); err != nil {
			parent.Close()
			return err
		}
	}
	stage, err := openRealDirectory(parent, stageName(p))
	if err == nil {
		check := readDirectoryOwner(ctx, stage, transfer.OwnerFile, p)
		if errors.Is(check, os.ErrNotExist) {
			f, e := stage.Open(".")
			if e != nil {
				stage.Close()
				parent.Close()
				return e
			}
			entries, e := f.ReadDir(1)
			f.Close()
			if (e == nil || errors.Is(e, io.EOF)) && len(entries) == 0 {
				check = nil
			}
		}
		if check != nil {
			stage.Close()
			parent.Close()
			return check
		}
		name := stage.Name()
		stage.Close()
		if err := trash(name); err != nil {
			parent.Close()
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		parent.Close()
		return err
	}
	parent.Close()
	central, err := s.central(id)
	if err != nil {
		return err
	}
	name := central.Name()
	if p.Kind == "import" {
		central.Close()
		return trash(name)
	}
	defer central.Close()
	return central.Remove(directoryPending)
}

var _ app.DirectoryRepository = (*DirectoryTransferStore)(nil)

func finishBackupCleanup(ctx context.Context, history *os.Root, state directoryState) error {
	if err := writeDirectoryJSON(ctx, history, state.Plan.Token+".json", state); err != nil {
		return err
	}
	if err := history.Remove(state.Plan.Token + ".json"); err != nil {
		return err
	}
	return fileio.SyncDirectory(history)
}
func (s *DirectoryTransferStore) checkHistoryCapacity(ctx context.Context, id string) error {
	root, err := s.central(id)
	if err != nil {
		return err
	}
	defer root.Close()
	history, err := openRealDirectory(root, "directory-history")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer history.Close()
	f, err := history.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	entries, err := f.ReadDir(257)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) >= 256 {
		return errors.New("本实例已保留256条迁移记录，请先清理旧副本")
	}
	return ctx.Err()
}
