package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"

	"mcl/internal/core/loader"
	"mcl/internal/fileio"
)

type loaderAliasDocument struct {
	Schema               int
	Requested, Canonical loader.Request
}

func (s *LoaderStore) LoadRequestAlias(ctx context.Context, requested loader.Request) (loader.Request, error) {
	if err := requested.Validate(); err != nil {
		return requested, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return requested, err
	}
	defer root.Close()
	var document loaderAliasDocument
	err = readUpdateJSON(ctx, root, path.Join("cache", "loader-aliases", requested.Key()+".json"), 8<<10, &document)
	if errors.Is(err, os.ErrNotExist) {
		return requested, nil
	}
	if err != nil {
		return requested, err
	}
	if document.Schema != 1 || document.Requested != requested {
		return requested, errors.New("Forge 构建映射与当前请求身份不符")
	}
	if err := loader.ValidateForgeAlias(requested, document.Canonical); err != nil {
		return requested, err
	}
	return document.Canonical, ctx.Err()
}

// The alias is published only once its fully keyed profile and runtime have
// passed their normal validation. It never replaces an instance/pack version.
func (s *LoaderStore) SaveRequestAlias(ctx context.Context, requested, canonical loader.Request) error {
	if err := loader.ValidateForgeAlias(requested, canonical); err != nil {
		return err
	}
	if requested == canonical {
		return ctx.Err()
	}
	if _, err := s.Load(ctx, canonical); err != nil {
		return err
	}
	data, err := json.Marshal(loaderAliasDocument{Schema: 1, Requested: requested, Canonical: canonical})
	if err != nil {
		return err
	}
	if len(data) > 8<<10 {
		return errors.New("Forge 构建映射超过限制")
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := fileio.WriteAt(ctx, root, path.Join("cache", "loader-aliases", requested.Key()+".json"), data, 0600); err != nil {
		return err
	}
	directory, err := root.OpenRoot("cache/loader-aliases")
	if err != nil {
		return err
	}
	defer directory.Close()
	return fileio.SyncDirectory(directory)
}
