package storage

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/transfer"
	"mcl/internal/fileio"
)

func directoryMove(source *os.Root, old string, target *os.Root, name string) error {
	if err := fileio.MoveNoReplace(source, old, target, name); err != nil {
		return err
	}
	return errors.Join(fileio.SyncDirectory(target), fileio.SyncDirectory(source))
}
func (s *DirectoryTransferStore) deletionBackups(ctx context.Context, id string) ([]app.InstanceDeletionBackup, []byte, error) {
	root, err := s.central(id)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	history, err := openRealDirectory(root, "directory-history")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	defer history.Close()
	f, err := history.Open(".")
	if err != nil {
		return nil, nil, err
	}
	entries, err := f.ReadDir(257)
	f.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, nil, err
	}
	if len(entries) > 256 {
		return nil, nil, errors.New("迁移历史超出删除核验边界，未回收任何文件")
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	var backups []app.InstanceDeletionBackup
	var digests []byte
	for _, entry := range entries {
		token := strings.TrimSuffix(entry.Name(), ".json")
		if !instance.ValidID(token) || entry.Name() != token+".json" {
			return nil, nil, errors.New("迁移历史含未知文件，请先核对")
		}
		var state directoryState
		if err := readUpdateJSON(ctx, history, entry.Name(), 32<<20, &state); err != nil {
			return nil, nil, err
		}
		if err := validateDirectoryState(id, state); err != nil {
			return nil, nil, err
		}
		if state.BackupGone {
			continue
		}
		if state.Plan.Token != token || state.Phase != "complete" || state.Plan.Kind != "move" {
			return nil, nil, errors.New("迁移副本记录不完整，请先在目录任务中恢复")
		}
		parent, err := s.parent(ctx, state.Plan.Before.GameDirectoryID, id)
		if err != nil {
			return nil, nil, err
		}
		value := app.InstanceDeletionBackup{Path: parent.Name() + string(os.PathSeparator) + backupName(state.Plan)}
		backup, err := openRealDirectory(parent, backupName(state.Plan))
		parent.Close()
		if errors.Is(err, os.ErrNotExist) {
			value.Gone = true
		} else if err != nil {
			return nil, nil, err
		} else {
			err = readDirectoryOwner(ctx, backup, directoryBackupOwner, state.Plan)
			backup.Close()
			if err != nil {
				return nil, nil, err
			}
		}
		data, _ := json.Marshal(state)
		sum := sha256.Sum256(data)
		digests = append(digests, sum[:]...)
		backups = append(backups, value)
	}
	return backups, digests, ctx.Err()
}
func checkStageTop(root *os.Root, allowed []string) error {
	f, err := root.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	entries, err := f.ReadDir(len(allowed) + 2)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != transfer.OwnerFile && !slices.Contains(allowed, entry.Name()) {
			return errors.New("暂存载荷出现未批准内容，已保留")
		}
	}
	return nil
}
func (s *DirectoryTransferStore) recordImport(ctx context.Context, plan app.DirectoryPlan) error {
	root, err := s.central(plan.After.ID)
	if err != nil {
		return err
	}
	defer root.Close()
	// Local source paths are only used by the live recovery journal. The final
	// provenance retains registration/candidate identity, not a private path as a
	// launch reference, nor foreign launch metadata or launcher configuration.
	record := struct {
		Schema                                int
		SourceID, CandidateID, MetadataSHA256 string
		Game                                  instance.Instance
		Tree                                  transfer.Tree
	}{1, plan.SourceID, plan.CandidateID, plan.SourceFingerprint, plan.After, plan.Tree}
	return writeDirectoryJSON(ctx, root, "directory-import.json", record)
}
