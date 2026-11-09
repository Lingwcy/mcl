package app

import (
	"context"
	"fmt"
	"strings"
)

// GameDirectory describes an installation location, not a migration of launcher
// data or a shared Minecraft working directory. Empty ID is the legacy location.
type GameDirectory struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Problem   string `json:"-"`
	Instances int    `json:"-"`
}

type GameDirectoryRepository interface {
	List(context.Context) ([]GameDirectory, error)
	Directory(context.Context, string) (string, error)
	Add(context.Context, string) (GameDirectory, error)
	Rename(context.Context, string, string) error
	Remove(context.Context, string) error
}

type GameDirectoryService struct{ repository GameDirectoryRepository }

func NewGameDirectoryService(repository GameDirectoryRepository) *GameDirectoryService {
	return &GameDirectoryService{repository: repository}
}
func (s *GameDirectoryService) List(ctx context.Context) ([]GameDirectory, error) {
	return s.repository.List(ctx)
}
func (s *GameDirectoryService) Add(ctx context.Context, directory string) (GameDirectory, error) {
	value, err := s.repository.Add(ctx, directory)
	if err != nil {
		return value, fmt.Errorf("添加游戏目录: %w", err)
	}
	return value, nil
}
func (s *GameDirectoryService) Rename(ctx context.Context, id, name string) error {
	return s.repository.Rename(ctx, id, strings.TrimSpace(name))
}
func (s *GameDirectoryService) Remove(ctx context.Context, id string) error {
	return s.repository.Remove(ctx, id)
}

func (s *GameDirectoryService) Directory(ctx context.Context, id string) (string, error) {
	return s.repository.Directory(ctx, id)
}
