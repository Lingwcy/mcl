package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"mcl/internal/app"
	"mcl/internal/fileio"
	"mcl/internal/platform/instancelock"
)

const accountsVersion = 1

type AccountStore struct{ root string }

func NewAccountStore(root string) *AccountStore { return &AccountStore{root: root} }

type accountsDocument struct {
	Version int `json:"version"`
	app.Accounts
}

func (s *AccountStore) Lock(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lock, err := instancelock.AcquireAccounts(s.root)
	if errors.Is(err, instancelock.ErrBusy) {
		return nil, errors.New("另一个 mcl 窗口正在修改账户，请稍后重试")
	}
	if err != nil {
		return nil, err
	}
	return func() { _ = lock.Close() }, nil
}

func (s *AccountStore) Load(ctx context.Context) (app.Accounts, error) {
	data, err := fileio.Read(ctx, filepath.Join(s.root, "accounts.json"), 256<<10)
	if errors.Is(err, os.ErrNotExist) {
		return app.Accounts{}, nil
	}
	if err != nil {
		return app.Accounts{}, err
	}
	var document accountsDocument
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&document) != nil || !errors.Is(d.Decode(new(any)), io.EOF) || document.Version != accountsVersion {
		return app.Accounts{}, errors.New("无法读取账户配置，已保留原文件")
	}
	return document.Accounts, document.Accounts.Validate()
}

func (s *AccountStore) Save(ctx context.Context, accounts app.Accounts) error {
	if err := accounts.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(accountsDocument{accountsVersion, accounts}, "", "  ")
	if err != nil {
		return err
	}
	return fileio.Write(ctx, filepath.Join(s.root, "accounts.json"), append(data, '\n'), 0o600)
}
