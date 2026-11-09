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
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"mcl/internal/app"
	"mcl/internal/core/discovery"
	"mcl/internal/core/instance"
	"mcl/internal/fileio"
	"mcl/internal/platform/instancelock"
)

type existingSourcesDocument struct {
	Schema  int                  `json:"schema"`
	Sources []app.ExistingSource `json:"sources"`
}

// ExistingGamesStore writes registration JSON ONLY below the application root.
// No source owner, receipt, lock, launcher config, or instance record is written.
type ExistingGamesStore struct{ root string }

func NewExistingGamesStore(root string) *ExistingGamesStore { return &ExistingGamesStore{root} }
func validExistingPath(value string) bool {
	return filepath.IsAbs(value) && filepath.Clean(value) == value && len(value) <= 4096 && utf8.ValidString(value) && !strings.ContainsFunc(value, unicode.IsControl)
}
func (s *ExistingGamesStore) read(ctx context.Context) (existingSourcesDocument, error) {
	var doc existingSourcesDocument
	data, err := fileio.Read(ctx, filepath.Join(s.root, "existing-sources.json"), 256<<10)
	if errors.Is(err, os.ErrNotExist) {
		return existingSourcesDocument{Schema: 1}, nil
	}
	if err != nil {
		return doc, err
	}
	if err := decodeGameJSON(data, &doc); err != nil {
		return doc, fmt.Errorf("只读来源登记损坏，原文件已保留: %w", err)
	}
	if doc.Schema != 1 || len(doc.Sources) > 32 {
		return doc, errors.New("只读来源登记版本或数量无效，原文件已保留")
	}
	seen := make(map[string]bool)
	for i, value := range doc.Sources {
		if !instance.ValidID(value.ID) || !validExistingPath(value.Path) || value.Name == "" || len(value.Name) > 256 || strings.ContainsFunc(value.Name, unicode.IsControl) || seen[value.ID] {
			return doc, errors.New("只读来源登记含无效位置或身份")
		}
		seen[value.ID] = true
		for _, previous := range doc.Sources[:i] {
			if sourcePathsOverlap(previous.Path, value.Path) {
				return doc, errors.New("只读来源位置重复或相互重叠")
			}
		}
	}
	return doc, ctx.Err()
}
func sourcePathsOverlap(a, b string) bool {
	a, b = strings.ToLower(norm.NFC.String(a)), strings.ToLower(norm.NFC.String(b))
	return pathContains(a, b) || pathContains(b, a)
}
func (s *ExistingGamesStore) List(ctx context.Context) ([]app.ExistingSource, error) {
	doc, err := s.read(ctx)
	return slices.Clone(doc.Sources), err
}
func (s *ExistingGamesStore) write(ctx context.Context, doc existingSourcesDocument) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return fileio.Write(ctx, filepath.Join(s.root, "existing-sources.json"), append(data, 10), 0o600)
}
func openExistingRoot(directory string) (*os.Root, error) {
	canonical, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, err
	}
	if filepath.Clean(canonical) != directory {
		return nil, errors.New("来源目录的链接或规范路径已改变，请重新选择")
	}
	before, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("来源不是普通目录")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	after, pathErr := os.Lstat(directory)
	if err != nil || pathErr != nil || !after.IsDir() || !os.SameFile(before, opened) || !os.SameFile(before, after) {
		root.Close()
		return nil, errors.New("打开期间来源目录发生变化")
	}
	return root, nil
}
func hasVersions(root *os.Root) bool {
	info, err := root.Lstat("versions")
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

// Selecting a parent may resolve to exactly one immediate .minecraft/minecraft
// child. Never recursively search unrelated user folders or guess between two.
func normalizeExistingRoot(ctx context.Context, directory string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !validExistingPath(filepath.Clean(directory)) {
		return "", errors.New("请选择绝对路径的Minecraft目录")
	}
	canonical, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", err
	}
	canonical = filepath.Clean(canonical)
	root, err := openExistingRoot(canonical)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if hasVersions(root) {
		return canonical, nil
	}
	var choices []string
	for _, name := range []string{".minecraft", "minecraft"} {
		info, err := root.Lstat(name)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		child, err := root.OpenRoot(name)
		if err != nil {
			continue
		}
		opened, statErr := child.Stat(".")
		if statErr == nil && os.SameFile(info, opened) && hasVersions(child) {
			choices = append(choices, filepath.Join(canonical, name))
		}
		child.Close()
	}
	if len(choices) > 1 {
		return "", errors.New("同时发现.minecraft和minecraft，请直接选择要扫描的目录")
	}
	if len(choices) == 1 {
		return choices[0], ctx.Err()
	}
	return "", errors.New("所选位置没有普通versions目录；请选择Minecraft根目录")
}

// Resolve the nearest existing ancestor too: --data-dir may not yet exist
// below a symlink. Creating its registry must still never write in a source.
func canonicalExistingDataRoot(directory string) (string, error) {
	current, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func (s *ExistingGamesStore) Add(ctx context.Context, directory string) (app.ExistingSource, error) {
	var value app.ExistingSource
	canonical, err := normalizeExistingRoot(ctx, directory)
	if err != nil {
		return value, err
	}
	protected, err := canonicalExistingDataRoot(s.root)
	if err != nil {
		return value, err
	}
	if sourcePathsOverlap(canonical, protected) {
		return value, errors.New("不能将mcl应用数据目录或其父子目录登记为外部来源")
	}
	lock, err := instancelock.AcquireExistingSources(s.root)
	if err != nil {
		return value, err
	}
	defer lock.Close()
	doc, err := s.read(ctx)
	if err != nil {
		return value, err
	}
	if len(doc.Sources) >= 32 {
		return value, errors.New("最多登记32个只读来源")
	}
	for _, previous := range doc.Sources {
		if sourcePathsOverlap(previous.Path, canonical) {
			return value, errors.New("来源已登记或与另一来源重叠")
		}
		a, aErr := os.Stat(previous.Path)
		b, bErr := os.Stat(canonical)
		if aErr == nil && bErr == nil && os.SameFile(a, b) {
			return value, errors.New("这两个路径实际指向同一目录")
		}
	}
	value = app.ExistingSource{ID: instance.NewID(), Name: filepath.Base(canonical), Path: canonical}
	doc.Sources = append(doc.Sources, value)
	if err := s.write(ctx, doc); err != nil {
		return app.ExistingSource{}, err
	}
	return value, nil
}
func (s *ExistingGamesStore) Remove(ctx context.Context, id string) error {
	if !instance.ValidID(id) {
		return errors.New("只读来源ID无效")
	}
	lock, err := instancelock.AcquireExistingSources(s.root)
	if err != nil {
		return err
	}
	defer lock.Close()
	doc, err := s.read(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(doc.Sources, func(value app.ExistingSource) bool { return value.ID == id }) {
		return errors.New("只读来源已不在登记中")
	}
	doc.Sources = slices.DeleteFunc(doc.Sources, func(value app.ExistingSource) bool { return value.ID == id })
	return s.write(ctx, doc)
}
func (s *ExistingGamesStore) Scan(ctx context.Context, id string) (discovery.Report, error) {
	doc, err := s.read(ctx)
	if err != nil {
		return discovery.Report{}, err
	}
	for _, source := range doc.Sources {
		if source.ID != id {
			continue
		}
		root, err := openExistingRoot(source.Path)
		if err != nil {
			return discovery.Report{Root: source.Path, Problems: []string{"来源不可读取（可能离线、移动或链接改变）：" + err.Error()}}, ctx.Err()
		}
		defer root.Close()
		result, err := discovery.Scan(ctx, root)
		current, pathErr := os.Lstat(source.Path)
		opened, openErr := root.Stat(".")
		if pathErr != nil || openErr != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(current, opened) {
			return discovery.Report{}, errors.New("扫描期间来源根目录发生变化，结果已丢弃")
		}
		return result, err
	}
	return discovery.Report{}, errors.New("只读来源未登记")
}
