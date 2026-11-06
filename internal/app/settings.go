package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"mcl/internal/core/launch"
)

// Settings contains user preferences only. Account credentials belong in the
// system credential store, never in this model.
type Settings struct {
	JavaPath string `json:"java_path"`
	// Legacy heap values remain readable but launch preparation uses automatic
	// allocation for every instance. New configurations leave these at zero.
	MaxMemoryMiB int           `json:"max_memory_mib"`
	MinMemoryMiB int           `json:"min_memory_mib,omitempty"`
	GCMode       launch.GCMode `json:"gc_mode,omitempty"`
	JVMArguments string        `json:"jvm_arguments,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{GCMode: launch.GCTunedG1}
}

func (s Settings) Validate() error {
	if s.MaxMemoryMiB != 0 && (s.MaxMemoryMiB < 512 || s.MaxMemoryMiB > 65536) {
		return errors.New("游戏内存应在 512 到 65536 MiB 之间")
	}
	if s.MinMemoryMiB < 0 || s.MinMemoryMiB > s.MaxMemoryMiB {
		return errors.New("初始内存应在 0 到最大内存之间，0 表示由 JVM 决定")
	}
	if strings.ContainsRune(s.JavaPath, '\x00') {
		return errors.New("Java 路径包含无效字符")
	}
	if s.JavaPath != "" && !filepath.IsAbs(s.JavaPath) {
		return errors.New("请输入 Java 可执行文件的绝对路径")
	}
	args, err := launch.ParseJVMArguments(s.JVMArguments)
	if err != nil {
		return err
	}
	return launch.ValidateJVMArguments(s.GCMode, args)
}

// SettingsRepository is the persistence required by the settings use case.
type SettingsRepository interface {
	Save(context.Context, Settings) error
}

type SettingsService struct {
	repository SettingsRepository
}

func NewSettingsService(repository SettingsRepository) *SettingsService {
	return &SettingsService{repository: repository}
}

func (s *SettingsService) Save(ctx context.Context, settings Settings) error {
	settings.JavaPath = strings.TrimSpace(settings.JavaPath)
	if err := settings.Validate(); err != nil {
		return err
	}
	if err := s.repository.Save(ctx, settings); err != nil {
		return fmt.Errorf("保存设置失败: %w", err)
	}
	return nil
}
