package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"mcl/internal/app"
	"mcl/internal/fileio"
)

const (
	settingsVersion = 1
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
	data, err := fileio.Read(ctx, s.path, maxSettingsSize)
	if errors.Is(err, os.ErrNotExist) {
		return app.DefaultSettings(), nil
	}
	if err != nil {
		return app.Settings{}, fmt.Errorf("read settings: %w", err)
	}
	var document settingsDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return app.Settings{}, fmt.Errorf("decode settings: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return app.Settings{}, errors.New("settings contains trailing content")
	}
	if document.Version != settingsVersion {
		return app.Settings{}, fmt.Errorf("unsupported settings version %d", document.Version)
	}
	if err := document.Settings.Validate(); err != nil {
		return app.Settings{}, fmt.Errorf("invalid settings: %w", err)
	}
	return document.Settings, nil
}

func (s *SettingsStore) Save(ctx context.Context, settings app.Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	// Do not replace a corrupt or newer document, even if it changed after startup.
	if _, err := s.Load(ctx); err != nil {
		return fmt.Errorf("preserve unreadable settings: %w", err)
	}
	data, err := json.MarshalIndent(settingsDocument{settingsVersion, settings}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	return fileio.Write(ctx, s.path, append(data, '\n'), 0o600)
}
