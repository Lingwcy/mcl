package storage

import (
	"context"
	"errors"
	"mcl/internal/core/instance"
)

func (s *DirectoryTransferStore) BackupDirectory(ctx context.Context, id, token string) (string, error) {
	if !instance.ValidID(id) || !instance.ValidID(token) {
		return "", errors.New("备份身份无效")
	}
	root, err := s.central(id)
	if err != nil {
		return "", err
	}
	defer root.Close()
	history, err := openRealDirectory(root, "directory-history")
	if err != nil {
		return "", err
	}
	defer history.Close()
	var state directoryState
	if err := readUpdateJSON(ctx, history, token+".json", 32<<20, &state); err != nil {
		return "", err
	}
	if err := validateDirectoryState(id, state); err != nil {
		return "", err
	}
	if state.Plan.Token != token || state.Plan.Kind != "move" || state.Phase != "complete" || state.BackupGone {
		return "", errors.New("迁移旧副本记录不可用")
	}
	parent, err := s.parent(ctx, state.Plan.Before.GameDirectoryID, id)
	if err != nil {
		return "", err
	}
	defer parent.Close()
	backup, err := openRealDirectory(parent, backupName(state.Plan))
	if err != nil {
		return "", err
	}
	defer backup.Close()
	if err := readDirectoryOwner(ctx, backup, directoryBackupOwner, state.Plan); err != nil {
		return "", err
	}
	return backup.Name(), ctx.Err()
}
