package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mcl/internal/app"
	"mcl/internal/fileio"
	"mcl/internal/platform/instancelock"
)

const (
	settingsVersion = 6
	maxSettingsSize = 1 << 20
)

type settingsDocument struct {
	Version  int          `json:"version"`
	Settings app.Settings `json:"settings"`
}

// SettingsStore saves a single versioned document. Its caller serializes saves.
type SettingsStore struct {
	path string
}

func NewSettingsStore(path string) *SettingsStore {
	return &SettingsStore{path: path}
}

func (s *SettingsStore) Load(ctx context.Context) (app.Settings, error) {
	document, _, err := s.readDocument(ctx)
	return document.Settings, err
}

func (s *SettingsStore) readDocument(ctx context.Context) (settingsDocument, []byte, error) {
	data, err := fileio.Read(ctx, s.path, maxSettingsSize)
	if errors.Is(err, os.ErrNotExist) {
		return settingsDocument{Version: settingsVersion, Settings: app.DefaultSettings()}, nil, nil
	}
	if err != nil {
		return settingsDocument{}, nil, fmt.Errorf("read settings: %w", err)
	}
	var document settingsDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return settingsDocument{}, nil, fmt.Errorf("decode settings: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return settingsDocument{}, nil, errors.New("settings contains trailing content")
	}
	if document.Version < 1 || document.Version > settingsVersion {
		return settingsDocument{}, nil, fmt.Errorf("unsupported settings version %d", document.Version)
	}
	if err := document.Settings.Validate(); err != nil {
		return settingsDocument{}, nil, fmt.Errorf("invalid settings: %w", err)
	}
	return document, data, nil
}

func (s *SettingsStore) Save(ctx context.Context, settings app.Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	lock, err := instancelock.AcquireGameDirectories(filepath.Dir(s.path))
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := NewGameDirectoryStore(filepath.Dir(s.path)).Registered(ctx, settings.GameDirectoryID); err != nil {
		return err
	}
	// Do not replace a corrupt or newer document, even if it changed after startup.
	previous, original, err := s.readDocument(ctx)
	if err != nil {
		return fmt.Errorf("preserve unreadable settings: %w", err)
	}
	data, err := json.MarshalIndent(settingsDocument{settingsVersion, settings}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if previous.Version < settingsVersion {
		if err := s.backupLegacy(ctx, original, previous.Version); err != nil {
			return fmt.Errorf("备份旧版设置: %w", err)
		}
	}
	return fileio.Write(ctx, s.path, append(data, '\n'), 0o600)
}

// Keep a private, non-overwriting recovery copy before introducing a schema
// that an earlier launcher cannot read. A failed backup prevents the upgrade.
func (s *SettingsStore) backupLegacy(ctx context.Context, data []byte, version int) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), fmt.Sprintf("settings-v%d-*.json.bak", version))
	if err != nil {
		return err
	}
	defer func() {
		file.Close()
		if err != nil {
			os.Remove(file.Name())
		}
	}()
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return ctx.Err()
}
