package app

import (
	"context"
	"fmt"

	"mcl/internal/core/discovery"
)

// ExistingSource is a read-only discovery root, never a GameDirectory write
// location. Registering/removing it must not adopt/delete any foreign files.
type ExistingSource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}
type ExistingGamesRepository interface {
	List(context.Context) ([]ExistingSource, error)
	Add(context.Context, string) (ExistingSource, error)
	Remove(context.Context, string) error
	Scan(context.Context, string) (discovery.Report, error)
}
type ExistingGamesService struct {
	store    ExistingGamesRepository
	scanSlot chan struct{}
}

func NewExistingGamesService(store ExistingGamesRepository) *ExistingGamesService {
	return &ExistingGamesService{store: store, scanSlot: make(chan struct{}, 1)}
}
func (s *ExistingGamesService) List(ctx context.Context) ([]ExistingSource, error) {
	return s.store.List(ctx)
}
func (s *ExistingGamesService) Add(ctx context.Context, directory string) (ExistingSource, error) {
	value, err := s.store.Add(ctx, directory)
	if err != nil {
		return value, fmt.Errorf("添加来源: %w", err)
	}
	return value, nil
}
func (s *ExistingGamesService) Remove(ctx context.Context, id string) error {
	return s.store.Remove(ctx, id)
}
func (s *ExistingGamesService) Scan(ctx context.Context, id string) (discovery.Report, error) {
	select {
	case s.scanSlot <- struct{}{}:
		defer func() { <-s.scanSlot }()
	case <-ctx.Done():
		return discovery.Report{}, ctx.Err()
	}
	return s.store.Scan(ctx, id)
}
