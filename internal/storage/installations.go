package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"

	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/fileio"
)

type installationDocument struct {
	Schema  int             `json:"schema"`
	Receipt install.Receipt `json:"receipt"`
}

type InstallationStore struct{ root string }

func NewInstallationStore(root string) *InstallationStore { return &InstallationStore{root: root} }

func (s *InstallationStore) Load(ctx context.Context, id string) (install.Receipt, error) {
	if !instance.ValidID(id) {
		return install.Receipt{}, errors.New("invalid instance ID")
	}
	if err := ctx.Err(); err != nil {
		return install.Receipt{}, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return install.Receipt{}, err
	}
	defer root.Close()
	f, err := root.Open(path.Join("instances", id, "installation.json"))
	if err != nil {
		return install.Receipt{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return install.Receipt{}, err
	}
	if len(data) > 64<<10 {
		return install.Receipt{}, errors.New("installation receipt exceeds 64 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var doc installationDocument
	if err := d.Decode(&doc); err != nil {
		return install.Receipt{}, err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return install.Receipt{}, errors.New("installation receipt has trailing content")
	}
	if doc.Schema != 1 || doc.Receipt.InstanceID != id {
		return install.Receipt{}, errors.New("installation schema or ID mismatch")
	}
	if err := doc.Receipt.Validate(); err != nil {
		return install.Receipt{}, err
	}
	return doc.Receipt, ctx.Err()
}

func (s *InstallationStore) Save(ctx context.Context, receipt install.Receipt) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer root.Close()
	// Do not silently overwrite a corrupted or future-schema document.
	if _, err := s.Load(ctx, receipt.InstanceID); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.MarshalIndent(installationDocument{Schema: 1, Receipt: receipt}, "", "  ")
	if err != nil {
		return err
	}
	return fileio.WriteAt(ctx, root, path.Join("instances", receipt.InstanceID, "installation.json"), append(data, '\n'), 0o600)
}
