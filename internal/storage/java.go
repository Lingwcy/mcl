package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"mcl/internal/app"
	"mcl/internal/fileio"
	"mcl/internal/platform/instancelock"
)

type javaDocument struct {
	Version int `json:"version"`
	app.JavaRegistration
}

type JavaStore struct{ directory string }

func NewJavaStore(directory string) *JavaStore { return &JavaStore{directory: directory} }

func (s *JavaStore) Load(ctx context.Context) (app.JavaRegistration, error) {
	data, err := fileio.Read(ctx, filepath.Join(s.directory, "java.json"), 256<<10)
	if errors.Is(err, os.ErrNotExist) {
		return app.JavaRegistration{}, nil
	}
	if err != nil {
		return app.JavaRegistration{}, err
	}
	var document javaDocument
	if err := json.Unmarshal(data, &document); err != nil || document.Version != 1 {
		return app.JavaRegistration{}, errors.New("Java 列表无法读取，原文件已保留")
	}
	if err := validateJavaRegistration(document.JavaRegistration); err != nil {
		return app.JavaRegistration{}, err
	}
	return document.JavaRegistration, nil
}

func validateJavaRegistration(value app.JavaRegistration) error {
	if len(value.Paths) > 256 || len(value.Hidden) > 1024 {
		return errors.New("Java 注册数量超过限制")
	}
	for _, paths := range [][]string{value.Paths, value.Hidden} {
		for _, path := range paths {
			if !filepath.IsAbs(path) || len(path) > 4096 || strings.ContainsRune(path, '\x00') {
				return errors.New("Java 列表包含无效路径")
			}
		}
	}
	return nil
}

func (s *JavaStore) Register(ctx context.Context, executable string) error {
	return s.change(ctx, executable, false)
}

func (s *JavaStore) Hide(ctx context.Context, executable string) error {
	return s.change(ctx, executable, true)
}

func (s *JavaStore) change(ctx context.Context, executable string, hide bool) error {
	lock, err := instancelock.AcquireJava(s.directory)
	if err != nil {
		return fmt.Errorf("Java 列表正被其他操作使用: %w", err)
	}
	defer lock.Close()
	value, err := s.Load(ctx)
	if err != nil {
		return err
	}
	value.Paths = slices.DeleteFunc(value.Paths, func(path string) bool { return path == executable })
	value.Hidden = slices.DeleteFunc(value.Hidden, func(path string) bool { return path == executable })
	if hide {
		value.Hidden = append(value.Hidden, executable)
	} else {
		value.Paths = append(value.Paths, executable)
	}
	if err := validateJavaRegistration(value); err != nil {
		return err
	}
	data, err := json.MarshalIndent(javaDocument{1, value}, "", "  ")
	if err != nil {
		return err
	}
	return fileio.Write(ctx, filepath.Join(s.directory, "java.json"), append(data, '\n'), 0o600)
}
