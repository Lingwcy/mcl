package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"

	"mcl/internal/core/loader"
	"mcl/internal/fileio"
)

type loaderDocument struct {
	Schema      int            `json:"schema"`
	Fingerprint string         `json:"fingerprint"`
	Profile     loader.Profile `json:"profile"`
}

// LoaderStore contains derived metadata, never an installation-success marker.
// Its key includes the official base metadata digest and exact loader version.
type LoaderStore struct{ root string }

func NewLoaderStore(root string) *LoaderStore { return &LoaderStore{root: root} }

func (s *LoaderStore) Load(ctx context.Context, request loader.Request) (loader.Profile, error) {
	if err := request.Validate(); err != nil {
		return loader.Profile{}, err
	}
	if err := ctx.Err(); err != nil {
		return loader.Profile{}, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return loader.Profile{}, err
	}
	defer root.Close()
	f, err := root.Open(path.Join("cache", "loaders", request.Key()+".json"))
	if err != nil {
		return loader.Profile{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if err != nil {
		return loader.Profile{}, err
	}
	if len(data) > 4<<20 {
		return loader.Profile{}, errors.New("加载器缓存超过 4 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document loaderDocument
	if err := decoder.Decode(&document); err != nil {
		return loader.Profile{}, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return loader.Profile{}, errors.New("加载器缓存包含尾随内容")
	}
	if document.Schema != 1 || document.Profile.Request != request || document.Fingerprint != document.Profile.Fingerprint() {
		return loader.Profile{}, errors.New("加载器缓存身份或摘要不匹配")
	}
	if err := document.Profile.Validate(); err != nil {
		return loader.Profile{}, err
	}
	if err := document.Profile.VerifyRuntime(ctx, root); err != nil {
		return loader.Profile{}, err
	}
	return document.Profile, ctx.Err()
}

func (s *LoaderStore) Save(ctx context.Context, profile loader.Profile) error {
	if err := profile.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(loaderDocument{Schema: 1, Fingerprint: profile.Fingerprint(), Profile: profile})
	if err != nil {
		return err
	}
	if len(data) > 4<<20 {
		return errors.New("加载器缓存超过 4 MiB")
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer root.Close()
	return fileio.WriteAt(ctx, root, path.Join("cache", "loaders", profile.Request.Key()+".json"), append(data, '\n'), 0o600)
}
