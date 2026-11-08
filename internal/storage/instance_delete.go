package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/fileio"
	"mcl/internal/platform/instancelock"
	"os"
	"path/filepath"
)

const instanceDeleteMarker = "instance-delete.pending.json"

type instanceDeleteState struct {
	Schema          int
	ID, Fingerprint string
	PayloadGone     bool
}
type InstanceDeletionStore struct{ root string }

func NewInstanceDeletionStore(root string) *InstanceDeletionStore {
	return &InstanceDeletionStore{root: root}
}
func (s *InstanceDeletionStore) Lock(ctx context.Context, id string) (io.Closer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return instancelock.AcquireInstanceDeletion(s.root, id)
}
func (s *InstanceDeletionStore) Inspect(ctx context.Context, id string) (app.InstanceDeletionPlan, error) {
	plan := app.InstanceDeletionPlan{InstanceID: id}
	if !instance.ValidID(id) {
		return plan, errors.New("invalid instance ID")
	}
	value, err := NewInstanceStore(filepath.Join(s.root, "instances")).Get(ctx, id)
	if err != nil {
		return plan, err
	}
	parent, err := os.OpenRoot(filepath.Join(s.root, "instances"))
	if err != nil {
		return plan, err
	}
	defer parent.Close()
	managed, err := openRealDirectory(parent, id)
	if err != nil {
		return plan, err
	}
	defer managed.Close()
	data, err := json.Marshal(value)
	if err != nil {
		return plan, err
	}
	digest := sha256.Sum256(data)
	plan.Name, plan.Fingerprint = value.Name, hex.EncodeToString(digest[:])
	plan.MetadataPath = managed.Name()
	var state instanceDeleteState
	err = readUpdateJSON(ctx, managed, instanceDeleteMarker, 2048, &state)
	if err == nil {
		if state.Schema != 1 || state.ID != id || state.Fingerprint != plan.Fingerprint {
			return plan, errors.New("删除恢复记录与实例不一致，未删除任何数据")
		}
		plan.Pending, plan.PayloadGone = true, state.PayloadGone
	} else if !errors.Is(err, os.ErrNotExist) {
		return plan, err
	}
	if value.GameDirectoryID == "" {
		plan.GamePath = filepath.Join(plan.MetadataPath, "game")
		return plan, ctx.Err()
	}
	location, err := NewGameDirectoryStore(s.root).open(ctx, value.GameDirectoryID)
	if err != nil {
		return plan, err
	}
	defer location.Close()
	plan.ExternalPayload = filepath.Join(location.Name(), id)
	plan.GamePath = filepath.Join(plan.ExternalPayload, "game")
	info, err := location.Lstat(id)
	if errors.Is(err, os.ErrNotExist) {
		plan.PayloadGone = true
		return plan, ctx.Err()
	}
	if err != nil {
		return plan, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return plan, errors.New("实例游戏载荷不是受管本地目录，未删除")
	}
	if plan.PayloadGone {
		return plan, errors.New("已移入废纸篓的游戏路径又出现了文件，未删除新数据")
	}
	checked, err := openRealDirectory(location, id)
	if err != nil {
		return plan, err
	}
	checked.Close()
	return plan, ctx.Err()
}
func (s *InstanceDeletionStore) Mark(ctx context.Context, expected app.InstanceDeletionPlan) error {
	current, err := s.Inspect(ctx, expected.InstanceID)
	if err != nil {
		return err
	}
	if current.Fingerprint != expected.Fingerprint || current.MetadataPath != expected.MetadataPath || current.ExternalPayload != expected.ExternalPayload {
		return errors.New("实例已改变，请重新确认删除")
	}
	return s.write(ctx, current, current.PayloadGone)
}
func (s *InstanceDeletionStore) PayloadRemoved(ctx context.Context, expected app.InstanceDeletionPlan) error {
	return s.write(ctx, expected, true)
}
func (s *InstanceDeletionStore) write(ctx context.Context, expected app.InstanceDeletionPlan, gone bool) error {
	parent, err := os.OpenRoot(filepath.Join(s.root, "instances"))
	if err != nil {
		return err
	}
	defer parent.Close()
	root, err := openRealDirectory(parent, expected.InstanceID)
	if err != nil {
		return err
	}
	defer root.Close()
	data, err := json.Marshal(instanceDeleteState{Schema: 1, ID: expected.InstanceID, Fingerprint: expected.Fingerprint, PayloadGone: gone})
	if err != nil {
		return err
	}
	return fileio.WriteAt(ctx, root, instanceDeleteMarker, data, 0600)
}
