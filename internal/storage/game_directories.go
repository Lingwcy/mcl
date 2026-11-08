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
	"strings"
	"unicode"
	"unicode/utf8"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/fileio"
	"mcl/internal/platform/instancelock"
)

const gameNamespace = "mcl-games-v1"

type gameDirectoriesDocument struct {
	Schema      int                 `json:"schema"`
	Owner       string              `json:"owner"`
	Directories []app.GameDirectory `json:"directories"`
}
type gameDirectoryOwner struct {
	DataRoot  string `json:"data_root"`
	Directory string `json:"directory"`
	Schema    int    `json:"schema"`
	Owner     string `json:"owner"`
	ID        string `json:"id"`
}

// GameDirectoryStore only manages explicitly registered, owned children of
// selected directories. The selected parent itself is never a game write root.
type GameDirectoryStore struct{ root string }

func NewGameDirectoryStore(root string) *GameDirectoryStore { return &GameDirectoryStore{root: root} }
func (s *GameDirectoryStore) read(ctx context.Context) (gameDirectoriesDocument, error) {
	var doc gameDirectoriesDocument
	data, err := fileio.Read(ctx, filepath.Join(s.root, "game-directories.json"), 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return doc, nil
	}
	if err != nil {
		return doc, err
	}
	if err = decodeGameJSON(data, &doc); err != nil {
		return doc, err
	}
	if doc.Schema != 1 || !instance.ValidID(doc.Owner) || len(doc.Directories) > 32 {
		return doc, errors.New("游戏目录配置格式无效，原文件已保留")
	}
	ids, paths := map[string]bool{}, map[string]bool{}
	for _, d := range doc.Directories {
		if !instance.ValidID(d.ID) || validateGameDirectoryName(d.Name) != nil || !validGameDirectoryPath(d.Path) || filepath.Clean(d.Path) != d.Path || filepath.Base(d.Path) != gameNamespace || strings.ContainsRune(d.Path, 0) || ids[d.ID] || paths[strings.ToLower(d.Path)] {
			return doc, errors.New("游戏目录配置包含无效或重复位置")
		}
		ids[d.ID], paths[strings.ToLower(d.Path)] = true, true
	}
	return doc, ctx.Err()
}
func decodeGameJSON(data []byte, value any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("游戏目录配置包含多余内容")
	}
	return nil
}
func (s *GameDirectoryStore) write(ctx context.Context, doc gameDirectoriesDocument) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return fileio.Write(ctx, filepath.Join(s.root, "game-directories.json"), append(data, '\n'), 0600)
}
func validateGameDirectoryName(name string) error {
	if name == "" || name != strings.TrimSpace(name) || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 80 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return errors.New("目录名称应为 1–80 个字符，不能包含控制字符或首尾空白")
	}
	return nil
}
func (s *GameDirectoryStore) Registered(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	doc, err := s.read(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(doc.Directories, func(d app.GameDirectory) bool { return d.ID == id }) {
		return errors.New("游戏目录未登记，请重新选择安装位置")
	}
	return nil
}
func (s *GameDirectoryStore) List(ctx context.Context) ([]app.GameDirectory, error) {
	doc, err := s.read(ctx)
	if err != nil {
		return nil, err
	}
	result := append([]app.GameDirectory{{Name: "内置目录", Path: filepath.Join(s.root, "instances")}}, doc.Directories...)
	values, err := NewInstanceStore(filepath.Join(s.root, "instances")).List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range result {
		for _, v := range values {
			if v.GameDirectoryID == result[i].ID {
				result[i].Instances++
			}
		}
		if result[i].ID == "" {
			continue
		}
		root, err := s.openLocation(ctx, doc, result[i].ID)
		if err != nil {
			result[i].Problem = err.Error()
		} else {
			root.Close()
		}
	}
	return result, ctx.Err()
}
func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func (s *GameDirectoryStore) Add(ctx context.Context, directory string) (app.GameDirectory, error) {
	var value app.GameDirectory
	if !validGameDirectoryPath(directory) {
		return value, errors.New("请选择绝对路径的本地目录")
	}
	parent, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return value, err
	}
	parent = filepath.Clean(parent)
	info, err := os.Stat(parent)
	if err != nil {
		return value, err
	}
	if !info.IsDir() {
		return value, errors.New("选择的位置不是目录")
	}
	managed := filepath.Join(parent, gameNamespace)
	if !validGameDirectoryPath(managed) {
		return value, errors.New("游戏目录路径过长或包含控制字符")
	}
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return value, err
	}
	protected, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		return value, err
	}
	if pathContains(protected, managed) || pathContains(managed, protected) {
		return value, errors.New("不能将应用数据或备份目录作为外部游戏位置")
	}
	lock, err := instancelock.AcquireGameDirectories(s.root)
	if err != nil {
		return value, err
	}
	defer lock.Close()
	doc, err := s.read(ctx)
	if err != nil {
		return value, err
	}
	for _, d := range doc.Directories {
		if pathContains(strings.ToLower(d.Path), strings.ToLower(managed)) || pathContains(strings.ToLower(managed), strings.ToLower(d.Path)) {
			return value, errors.New("这个位置已登记或与已有游戏目录重叠")
		}
	}
	if len(doc.Directories) >= 32 {
		return value, errors.New("最多登记 32 个外部游戏位置")
	}
	if doc.Owner == "" {
		doc.Schema, doc.Owner = 1, instance.NewID()
		if err := s.write(ctx, doc); err != nil {
			return value, err
		}
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return value, err
	}
	defer root.Close()
	// A parent lock coordinates namespace initialization between app data roots.
	lockRoot, err := root.OpenRoot(".")
	if err != nil {
		return value, err
	}
	initLock, err := instancelock.AcquireGameDirectoryInit(lockRoot)
	if err != nil {
		return value, err
	}
	defer initLock.Close()
	value = app.GameDirectory{ID: instance.NewID(), Name: filepath.Base(parent), Path: managed}
	if validateGameDirectoryName(value.Name) != nil {
		value.Name = "游戏目录"
	}
	if err = root.Mkdir(gameNamespace, 0700); err == nil {
		child, err := root.OpenRoot(gameNamespace)
		if err != nil {
			return value, err
		}
		data, _ := json.Marshal(gameDirectoryOwner{Schema: 1, Owner: doc.Owner, ID: value.ID, DataRoot: protected, Directory: managed})
		err = fileio.WriteAt(ctx, child, "owner.json", data, 0600)
		child.Close()
		if err != nil {
			return value, err
		}
	} else if errors.Is(err, os.ErrExist) {
		// Reattach only a namespace owned by this registry (e.g. removed empty
		// registration), never adopt another launcher's files or an unmarked child.
		child, err := openRealDirectory(root, gameNamespace)
		if err != nil {
			return value, err
		}
		defer child.Close()
		owner, err := readGameOwner(ctx, child)
		if err != nil {
			return value, err
		}
		if owner.Owner != doc.Owner || owner.DataRoot != protected || owner.Directory != managed {
			return value, errors.New("目录中的 mcl-games-v1 不属于当前应用数据，未接管任何文件")
		}
		value.ID = owner.ID
	} else {
		return value, err
	}
	for _, d := range doc.Directories {
		if d.ID == value.ID {
			return value, errors.New("该位置标记已由另一目录使用，未接管文件")
		}
	}
	doc.Directories = append(doc.Directories, value)
	if err := s.write(ctx, doc); err != nil {
		return value, fmt.Errorf("保存目录登记失败，受管目录已保留，可重选同一位置: %w", err)
	}
	return value, nil
}
func (s *GameDirectoryStore) Rename(ctx context.Context, id, name string) error {
	if id == "" {
		return errors.New("内置目录不能重命名")
	}
	if err := validateGameDirectoryName(name); err != nil {
		return err
	}
	lock, err := instancelock.AcquireGameDirectories(s.root)
	if err != nil {
		return err
	}
	defer lock.Close()
	doc, err := s.read(ctx)
	if err != nil {
		return err
	}
	for i := range doc.Directories {
		if doc.Directories[i].ID == id {
			doc.Directories[i].Name = name
			return s.write(ctx, doc)
		}
	}
	return errors.New("游戏目录已被移除，请刷新")
}
func (s *GameDirectoryStore) Remove(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("内置目录不能移除")
	}
	lock, err := instancelock.AcquireGameDirectories(s.root)
	if err != nil {
		return err
	}
	defer lock.Close()
	doc, err := s.read(ctx)
	if err != nil {
		return err
	}
	values, err := NewInstanceStore(filepath.Join(s.root, "instances")).List(ctx)
	if err != nil {
		return err
	}
	for _, v := range values {
		if v.GameDirectoryID == id {
			return errors.New("这个目录仍有关联实例，不能移除登记；不会删除或移动游戏文件")
		}
	}
	settings, err := NewSettingsStore(filepath.Join(s.root, "settings.json")).Load(ctx)
	if err != nil {
		return err
	}
	if settings.GameDirectoryID == id {
		return errors.New("请先选择其他默认游戏目录")
	}
	before := len(doc.Directories)
	doc.Directories = slices.DeleteFunc(doc.Directories, func(d app.GameDirectory) bool { return d.ID == id })
	if before == len(doc.Directories) {
		return errors.New("游戏目录已被移除，请刷新")
	}
	return s.write(ctx, doc)
}
func readGameOwner(ctx context.Context, root *os.Root) (gameDirectoryOwner, error) {
	var owner gameDirectoryOwner
	info, err := root.Lstat("owner.json")
	if err != nil {
		return owner, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return owner, errors.New("游戏目录所属标记无效")
	}
	f, err := root.Open("owner.json")
	if err != nil {
		return owner, err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil {
		return owner, err
	}
	if !os.SameFile(info, actual) {
		return owner, errors.New("游戏目录所属标记发生变化")
	}
	data, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return owner, err
	}
	if err = decodeGameJSON(data, &owner); err != nil {
		return owner, err
	}
	if !validGameDirectoryPath(owner.DataRoot) || !validGameDirectoryPath(owner.Directory) || owner.Schema != 1 || !instance.ValidID(owner.Owner) || !instance.ValidID(owner.ID) {
		return owner, errors.New("游戏目录所属标记不受支持")
	}
	return owner, ctx.Err()
}
func openRealDirectory(root *os.Root, name string) (*os.Root, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("游戏目录不能是符号链接或其他文件")
	}
	child, err := root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	after, err := child.Stat(".")
	if err != nil || !os.SameFile(info, after) {
		child.Close()
		return nil, errors.New("游戏目录在打开时发生变化")
	}
	return child, nil
}
func (s *GameDirectoryStore) openLocation(ctx context.Context, doc gameDirectoriesDocument, id string) (*os.Root, error) {
	for _, d := range doc.Directories {
		if d.ID != id {
			continue
		}
		parent := filepath.Dir(d.Path)
		resolved, err := filepath.EvalSymlinks(parent)
		if err != nil {
			return nil, fmt.Errorf("游戏位置不可用，请检查磁盘: %w", err)
		}
		if filepath.Clean(resolved) != parent {
			return nil, errors.New("游戏位置的父路径已变化，请检查磁盘或目录链接")
		}
		base, err := os.OpenRoot(parent)
		if err != nil {
			return nil, err
		}
		defer base.Close()
		root, err := openRealDirectory(base, gameNamespace)
		if err != nil {
			return nil, err
		}
		owner, err := readGameOwner(ctx, root)
		canonical, rootErr := filepath.EvalSymlinks(s.root)
		if err != nil || rootErr != nil || owner.Owner != doc.Owner || owner.ID != id || owner.DataRoot != canonical || owner.Directory != d.Path {
			root.Close()
			return nil, errors.New("游戏位置所属标记不匹配，已拒绝读取或修改")
		}
		return root, nil
	}
	return nil, errors.New("实例的游戏目录未登记，不能改用其他位置")
}
func (s *GameDirectoryStore) open(ctx context.Context, id string) (*os.Root, error) {
	doc, err := s.read(ctx)
	if err != nil {
		return nil, err
	}
	return s.openLocation(ctx, doc, id)
}

// lockLocation serializes creation/publication with registry removal. Runtime
// operations do not need this lease: locations containing instances cannot be
// removed, and their path/owner are immutable.
func lockGameLocation(ctx context.Context, dataRoot, id string) (*os.Root, func(), error) {
	lock, err := instancelock.AcquireGameDirectories(dataRoot)
	if err != nil {
		return nil, nil, err
	}
	root, err := NewGameDirectoryStore(dataRoot).open(ctx, id)
	if err != nil {
		lock.Close()
		return nil, nil, err
	}
	return root, func() { root.Close(); lock.Close() }, nil
}

func (s *GameDirectoryStore) Directory(ctx context.Context, id string) (string, error) {
	if id == "" {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		root, err := os.OpenRoot(s.root)
		if err != nil {
			return "", err
		}
		defer root.Close()
		if err := root.Mkdir("instances", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
		child, err := openRealDirectory(root, "instances")
		if err != nil {
			return "", err
		}
		defer child.Close()
		return child.Name(), ctx.Err()
	}
	root, err := s.open(ctx, id)
	if err != nil {
		return "", err
	}
	defer root.Close()
	return root.Name(), ctx.Err()
}

func validGameDirectoryPath(directory string) bool {
	return filepath.IsAbs(directory) && len(directory) <= 4096 && utf8.ValidString(directory) && strings.IndexFunc(directory, unicode.IsControl) < 0
}
