package app

import (
	"context"
	"strings"

	"mcl/internal/core/instance"
)

type RuntimeRepository interface {
	UpdateRuntime(context.Context, string, string, int) (instance.Instance, error)
	Rename(context.Context, string, string) (instance.Instance, error)
}

func (s *RuntimeSettingsService) Rename(ctx context.Context, id, name string) (instance.Instance, error) {
	name = strings.TrimSpace(name)
	if err := instance.ValidateName(name); err != nil {
		return instance.Instance{}, err
	}
	lease, err := s.registry.Reserve(ctx, id)
	if err != nil {
		return instance.Instance{}, err
	}
	defer lease.Close()
	return s.store.Rename(ctx, id, name)
}

type RuntimeSettingsService struct {
	store    RuntimeRepository
	registry SessionRegistry
}

func NewRuntimeSettingsService(store RuntimeRepository, registry SessionRegistry) *RuntimeSettingsService {
	return &RuntimeSettingsService{store: store, registry: registry}
}
func (s *RuntimeSettingsService) Save(ctx context.Context, id string, settings Settings) (instance.Instance, error) {
	settings.JavaPath = strings.TrimSpace(settings.JavaPath)
	if err := settings.Validate(); err != nil {
		return instance.Instance{}, err
	}
	lease, err := s.registry.Reserve(ctx, id)
	if err != nil {
		return instance.Instance{}, err
	}
	defer lease.Close()
	return s.store.UpdateRuntime(ctx, id, settings.JavaPath, settings.MaxMemoryMiB)
}
