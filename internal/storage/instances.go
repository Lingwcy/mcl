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
	"slices"

	"mcl/internal/core/instance"
	"mcl/internal/fileio"
)

type instanceDocument struct {
	Version  int               `json:"version"`
	Instance instance.Instance `json:"instance"`
}

// InstanceStore confines managed instance documents and game folders to its root.
// Callers serialize changes to the same instance.
type InstanceStore struct {
	dir string
}

func NewInstanceStore(dir string) *InstanceStore {
	return &InstanceStore{dir: dir}
}

func (s *InstanceStore) Get(ctx context.Context, id string) (instance.Instance, error) {
	if !instance.ValidID(id) {
		return instance.Instance{}, errors.New("invalid instance ID")
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return instance.Instance{}, err
	}
	defer root.Close()
	return readInstance(ctx, root, id)
}

func (s *InstanceStore) List(ctx context.Context) ([]instance.Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open instances: %w", err)
	}
	defer root.Close()
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	var instances []instance.Instance
	for _, entry := range entries {
		if !instance.ValidID(entry.Name()) {
			continue
		}
		value, err := readInstance(ctx, root, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("读取实例 %s 失败，原文件已保留: %w", entry.Name(), err)
		}
		instances = append(instances, value)
	}
	slices.SortFunc(instances, func(a, b instance.Instance) int {
		if order := b.CreatedAt.Compare(a.CreatedAt); order != 0 {
			return order
		}
		return bytes.Compare([]byte(a.ID), []byte(b.ID))
	})
	return instances, nil
}

func (s *InstanceStore) Create(ctx context.Context, value instance.Instance) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Mkdir(value.ID, 0o700); err != nil {
		return fmt.Errorf("create instance directory: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			root.RemoveAll(value.ID)
		}
	}()
	if value.GameDirectoryID == "" {
		if err := root.Mkdir(filepath.Join(value.ID, "game"), 0o700); err != nil {
			return err
		}
	} else {
		location, release, err := lockGameLocation(ctx, filepath.Dir(s.dir), value.GameDirectoryID)
		if err != nil {
			return err
		}
		defer release()
		// Never reuse or overwrite a pre-existing ID, even if it appears empty.
		if err := location.Mkdir(value.ID, 0o700); err != nil {
			return err
		}
		payload, err := openRealDirectory(location, value.ID)
		if err != nil {
			return err
		}
		defer payload.Close()
		if err := payload.Mkdir("game", 0o700); err != nil {
			return err
		}
		// On failure preserve the freshly reserved external ID rather than
		// recursively deleting a directory another process could have touched.
		if err := writeInstance(ctx, root, value); err != nil {
			return fmt.Errorf("实例登记失败，新的游戏目录已保留 %s: %w", payload.Name(), err)
		}
		committed = true
		return nil
	}
	if err := writeInstance(ctx, root, value); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *InstanceStore) Rename(ctx context.Context, id, name string) (instance.Instance, error) {
	if !instance.ValidID(id) {
		return instance.Instance{}, errors.New("invalid instance ID")
	}
	if err := instance.ValidateName(name); err != nil {
		return instance.Instance{}, err
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return instance.Instance{}, err
	}
	defer root.Close()
	value, err := readInstance(ctx, root, id)
	if err != nil {
		return instance.Instance{}, err
	}
	value.Name = name
	if err := writeInstance(ctx, root, value); err != nil {
		return instance.Instance{}, err
	}
	return value, nil
}

func (s *InstanceStore) UpdateRuntime(ctx context.Context, id, javaPath string, memory int) (instance.Instance, error) {
	if !instance.ValidID(id) {
		return instance.Instance{}, errors.New("invalid instance ID")
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return instance.Instance{}, err
	}
	defer root.Close()
	value, err := readInstance(ctx, root, id)
	if err != nil {
		return instance.Instance{}, err
	}
	value.JavaPath, value.MaxMemoryMiB = javaPath, memory
	if err := value.Validate(); err != nil {
		return instance.Instance{}, err
	}
	if err := writeInstance(ctx, root, value); err != nil {
		return instance.Instance{}, err
	}
	return value, nil
}

func readInstance(ctx context.Context, root *os.Root, id string) (instance.Instance, error) {
	if err := ctx.Err(); err != nil {
		return instance.Instance{}, err
	}
	f, err := root.Open(filepath.Join(id, "instance.json"))
	if err != nil {
		return instance.Instance{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return instance.Instance{}, err
	}
	if len(data) > 1<<20 {
		return instance.Instance{}, errors.New("instance document exceeds 1 MiB")
	}
	var document instanceDocument
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&document); err != nil {
		return instance.Instance{}, err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return instance.Instance{}, errors.New("instance document contains trailing content")
	}
	if (document.Version < 1 || document.Version > 3) || document.Instance.ID != id {
		return instance.Instance{}, errors.New("instance schema or ID mismatch")
	}
	if err := document.Instance.Validate(); err != nil {
		return instance.Instance{}, err
	}
	return document.Instance, ctx.Err()
}

func writeInstance(ctx context.Context, root *os.Root, value instance.Instance) error {
	data, err := json.MarshalIndent(instanceDocument{Version: instanceDocumentVersion(value), Instance: value}, "", "  ")
	if err != nil {
		return err
	}
	return fileio.WriteAt(ctx, root, filepath.Join(value.ID, "instance.json"), append(data, '\n'), 0o600)
}

// Only new external-location instances require the new document version.
func instanceDocumentVersion(value instance.Instance) int {
	if value.Description != "" || (value.Pack != nil && (value.Pack.Source == "multimc" || value.Pack.Name != "" || value.Pack.Version != "" || value.Pack.Author != "")) {
		return 3
	}
	if value.GameDirectoryID != "" {
		return 2
	}
	return 1
}
