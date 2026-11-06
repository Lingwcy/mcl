package app

import (
	"context"
	"errors"
	"time"

	"mcl/internal/core/market"
)

type AssetKind string

const (
	AssetMod          AssetKind = "mod"
	AssetShader       AssetKind = "shader"
	AssetResourcePack AssetKind = "resource-pack"
	AssetDataPack     AssetKind = "data-pack"
	AssetModpack      AssetKind = "modpack"
)

// AssetFile describes an existing local file. Enabled reflects its filename,
// not whether Minecraft has loaded it or supports its required loader.
type AssetFile struct {
	Path, Name, World  string
	Kind               AssetKind
	Size               int64
	ModifiedAt         time.Time
	Directory, Enabled bool
	// Origin is a matching installation record, not proof the file is intact.
	// A mutation verifies recorded hashes before operating on a managed file.
	Origin  *market.InstallFile
	Origins []market.InstallFile
}

type AssetSnapshot struct {
	InstanceID    string
	Files         []AssetFile
	ScannedAt     time.Time
	Truncated     bool
	Warning       string
	PendingUpdate *AssetUpdateRecord
}

type AssetRepository interface {
	List(context.Context, string) (AssetSnapshot, error)
	Directory(context.Context, string) (string, error)
	Change(context.Context, AssetMutation) (AssetChange, error)
	Restore(context.Context, string, string) error
}

type AssetAction string

const (
	AssetEnable  AssetAction = "enable"
	AssetDisable AssetAction = "disable"
	AssetRemove  AssetAction = "remove"
)

type AssetMutation struct {
	InstanceID string
	File       AssetFile
	Action     AssetAction
}

type AssetChange struct{ BackupID string }

type AssetsService struct {
	instances InstanceReader
	store     AssetRepository
	registry  SessionRegistry
}

func NewAssetsService(instances InstanceReader, store AssetRepository, registry SessionRegistry) *AssetsService {
	return &AssetsService{instances: instances, store: store, registry: registry}
}

func (s *AssetsService) Change(ctx context.Context, request AssetMutation) (AssetChange, error) {
	if s.registry == nil {
		return AssetChange{}, errors.New("实例会话保护未配置")
	}
	lease, err := s.registry.Reserve(ctx, request.InstanceID)
	if err != nil {
		return AssetChange{}, err
	}
	defer lease.Close()
	if _, err := s.instances.Get(ctx, request.InstanceID); err != nil {
		return AssetChange{}, err
	}
	return s.store.Change(ctx, request)
}

func (s *AssetsService) Restore(ctx context.Context, id, backup string) error {
	if s.registry == nil {
		return errors.New("实例会话保护未配置")
	}
	lease, err := s.registry.Reserve(ctx, id)
	if err != nil {
		return err
	}
	defer lease.Close()
	if _, err := s.instances.Get(ctx, id); err != nil {
		return err
	}
	return s.store.Restore(ctx, id, backup)
}

func (s *AssetsService) List(ctx context.Context, id string) (AssetSnapshot, error) {
	if _, err := s.instances.Get(ctx, id); err != nil {
		return AssetSnapshot{}, err
	}
	return s.store.List(ctx, id)
}

func (s *AssetsService) Directory(ctx context.Context, id string) (string, error) {
	if _, err := s.instances.Get(ctx, id); err != nil {
		return "", err
	}
	return s.store.Directory(ctx, id)
}
