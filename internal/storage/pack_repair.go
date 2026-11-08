package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"mcl/internal/app"
	"mcl/internal/core/archive"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
	"mcl/internal/core/modpack"
)

type packOrigin struct {
	Schema   int                      `json:"schema"`
	Request  app.ArchiveImportRequest `json:"request"`
	Manifest modpack.Manifest         `json:"manifest"`
	Optional []string                 `json:"selected_optional"`
	Resolved []modpack.File           `json:"resolved_files"`
	Archive  *market.File             `json:"archive,omitempty"`
}

func (s *ArchiveStore) InspectPackRepair(ctx context.Context, value instance.Instance) (app.PackRepairPlan, error) {
	plan := app.PackRepairPlan{InstanceID: value.ID}
	if err := value.Validate(); err != nil {
		return plan, err
	}
	pack := value.Pack
	if pack == nil {
		return plan, errors.New("此实例没有整合包清单")
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return plan, err
	}
	defer root.Close()
	origin, digest, err := readPackOrigin(root, value)
	if err != nil {
		return plan, err
	}
	plan.Receipt = digest
	// A legacy receipt can still use a locally cached original archive.
	if origin.Archive != nil {
		plan.Archive = *origin.Archive
	}
	archiveSHA := strings.ToLower(pack.ArchiveSHA1)
	info, err := root.Lstat(path.Join("cache", "market", archiveSHA))
	if err == nil && info.Mode().IsRegular() && info.Size() > 0 && info.Size() <= modpack.MaxArchiveSize && plan.Archive.Size == 0 {
		plan.Archive = market.File{SHA1: archiveSHA, Size: info.Size()}
	}
	game, err := s.assets.openGame(ctx, value.ID)
	if err != nil {
		return plan, err
	}
	defer game.Close()
	files := origin.Resolved
	if files == nil {
		if origin.Manifest.Format == "modrinth" || origin.Manifest.Format == "multimc" {
			files = (app.ArchiveImportPlan{Manifest: origin.Manifest, Files: origin.Manifest.Files, SelectedOptional: origin.Optional}).ActiveFiles()
		} else {
			plan.Legacy = len(origin.Manifest.Files) > 0
		}
	}
	index := newPackPathIndex(game)
	seen := make(map[string]bool, len(files)+len(origin.Manifest.Overrides))
	add := func(entry app.PackRepairEntry) error {
		key := archive.Key(entry.Path)
		if !archive.SafePath(entry.Path) || seen[key] {
			return errors.New("整合包来源记录包含无效或重复的文件路径")
		}
		seen[key] = true
		occupied, err := index.occupied(ctx, entry.Path)
		if err != nil {
			return err
		}
		if occupied {
			plan.Preserved++
		} else {
			plan.Entries = append(plan.Entries, entry)
		}
		return nil
	}
	for _, file := range files {
		if err := add(app.PackRepairEntry{Path: file.Path, File: file}); err != nil {
			return plan, err
		}
	}
	for _, override := range origin.Manifest.Overrides {
		if err := add(app.PackRepairEntry{Path: override.Target, Override: true}); err != nil {
			return plan, err
		}
	}
	return plan, ctx.Err()
}

func readPackOrigin(root *os.Root, value instance.Instance) (packOrigin, string, error) {
	var result packOrigin
	name := path.Join("instances", value.ID, "modpack.json")
	info, err := root.Lstat(name)
	if err != nil {
		return result, "", fmt.Errorf("读取整合包来源记录: %w", err)
	}
	const maxReceipt = 16 << 20
	if !info.Mode().IsRegular() || info.Size() > maxReceipt {
		return result, "", errors.New("整合包来源记录类型或大小无效")
	}
	file, err := root.Open(name)
	if err != nil {
		return result, "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return result, "", errors.New("整合包来源记录已改变")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxReceipt+1))
	if err != nil {
		return result, "", err
	}
	if len(data) > maxReceipt || int64(len(data)) != info.Size() {
		return result, "", errors.New("整合包来源记录大小已改变")
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, "", fmt.Errorf("解析整合包来源记录: %w", err)
	}
	pack := value.Pack
	if result.Schema != 1 || result.Request.Kind != market.Modpack || string(result.Request.Source) != pack.Source ||
		(result.Request.LocalArchive != nil) != pack.Local ||
		result.Request.ProjectID != pack.ProjectID || result.Request.VersionID != pack.VersionID ||
		result.Manifest.Format != pack.Source || result.Manifest.Minecraft != value.Version.ID ||
		len(result.Manifest.Files) > modpack.MaxFiles || len(result.Resolved) > modpack.MaxFiles || len(result.Manifest.Overrides) > archive.MaxEntries ||
		(result.Archive != nil && !strings.EqualFold(result.Archive.SHA1, pack.ArchiveSHA1)) {
		return result, "", errors.New("整合包来源记录与实例不匹配")
	}
	sum := sha256.Sum256(data)
	return result, hex.EncodeToString(sum[:]), nil
}

// Keep existing files and their disabled or portable-name aliases. Directory
// listings are shared by paths in the same parent during a single check.
type packPathIndex struct {
	root    *os.Root
	dirs    map[string]map[string]os.DirEntry
	entries int
}

func newPackPathIndex(root *os.Root) *packPathIndex {
	return &packPathIndex{root: root, dirs: make(map[string]map[string]os.DirEntry)}
}

func (i *packPathIndex) occupied(ctx context.Context, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	dir := "."
	parts := strings.Split(name, "/")
	for n, part := range parts {
		entries, err := i.list(dir)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		entry, found := entries[archive.Key(part)]
		if n == len(parts)-1 {
			_, disabled := entries[archive.Key(part+".disabled")]
			return found || disabled, nil
		}
		if !found {
			return false, nil
		}
		dir = path.Join(dir, part)
		if entry == nil || !entry.IsDir() {
			return true, nil
		}
		info, err := i.root.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			return true, nil // A differently spelled parent exists.
		}
		if err != nil {
			return false, err
		}
		if !info.IsDir() {
			return true, nil
		}
	}
	return false, nil
}

func (i *packPathIndex) list(name string) (map[string]os.DirEntry, error) {
	if entries, ok := i.dirs[name]; ok {
		return entries, nil
	}
	dir, err := localDirectory(i.root, name)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	const maxEntries = 200000
	items, err := dir.ReadDir(maxEntries - i.entries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	i.entries += len(items)
	if i.entries > maxEntries {
		return nil, errors.New("实例目录超过整合包检查上限")
	}
	entries := make(map[string]os.DirEntry, len(items))
	for _, entry := range items {
		key := archive.Key(entry.Name())
		if _, duplicate := entries[key]; duplicate {
			entries[key] = nil
		} else {
			entries[key] = entry
		}
	}
	i.dirs[name] = entries
	return entries, nil
}

func (s *ArchiveStore) CommitPackRepair(ctx context.Context, value instance.Instance, receipt string, original market.File, entries []app.PackRepairEntry) (err error) {
	game, err := s.assets.openGame(ctx, value.ID)
	if err != nil {
		return err
	}
	defer game.Close()
	cache, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer cache.Close()
	temp := ".mcl-pack-repair-" + rand.Text()
	if err := game.Mkdir(temp, 0o700); err != nil {
		return err
	}
	defer game.RemoveAll(temp)
	stage, err := game.OpenRoot(temp)
	if err != nil {
		return err
	}
	defer stage.Close()
	if err := s.stagePackRepair(ctx, cache, stage, original, entries); err != nil {
		return err
	}
	files := make([]market.InstallFile, 0, len(entries))
	var edits []localModEdit
	for _, entry := range entries {
		digest, size := entry.File.SHA1, entry.File.Size
		if entry.Override {
			file, err := stage.Open(entry.Path)
			if err != nil {
				return err
			}
			info, err := file.Stat()
			if err != nil {
				file.Close()
				return err
			}
			size = info.Size()
			result, err := market.Digest(ctx, file, size, false, nil)
			file.Close()
			if err != nil {
				return err
			}
			digest = result.SHA1
		}
		files = append(files, market.InstallFile{Path: entry.Path, File: market.File{Filename: entry.Path, SHA1: digest, Size: size}})
		if isLocalMod(entry.Path) {
			edits = append(edits, localModEdit{after: entry.Path, enabled: true, root: stage, source: entry.Path, size: size, sha1: digest})
		}
	}
	if err := s.assets.guardLocalMods(ctx, game, value.ID, "", edits); err != nil {
		return err
	}
	_, digest, err := readPackOrigin(cache, value)
	if err != nil {
		return err
	}
	if receipt != digest {
		return errors.New("整合包记录已改变，请重新检查")
	}
	index := newPackPathIndex(game)
	for _, entry := range entries {
		occupied, err := index.occupied(ctx, entry.Path)
		if err != nil {
			return err
		}
		if occupied {
			return fmt.Errorf("%s 已存在，请重新检查", entry.Path)
		}
	}
	published := 0
	defer func() {
		if err == nil || published == 0 {
			return
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		for n := published - 1; n >= 0; n-- {
			file := files[n]
			err = errors.Join(err, removeMarketPublication(cleanup, game, path.Join(temp, file.Path), file))
		}
	}()
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		dir := path.Dir(file.Path)
		if err := game.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		parent, err := localDirectory(game, dir)
		if err != nil {
			return err
		}
		parent.Close()
		if _, err := game.Lstat(file.Path + ".disabled"); !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return err
			}
			return fmt.Errorf("%s 的停用文件已存在，请重新检查", file.Path)
		}
		if err := game.Link(path.Join(temp, file.Path), file.Path); err != nil {
			return fmt.Errorf("无法恢复 %s: %w", file.Path, err)
		}
		published++
	}
	return ctx.Err()
}

func (s *ArchiveStore) stagePackRepair(ctx context.Context, cache, stage *os.Root, original market.File, entries []app.PackRepairEntry) error {
	overrides := make(map[string]bool)
	for _, entry := range entries {
		if entry.Override {
			overrides[entry.Path] = true
			continue
		}
		if err := stage.MkdirAll(path.Dir(entry.Path), 0o700); err != nil {
			return err
		}
		file := entry.File
		if err := copyVerifiedCache(ctx, cache, stage, entry.Path, file.SHA1, file.SHA512, file.Size); err != nil {
			return fmt.Errorf("准备 %s: %w", entry.Path, err)
		}
	}
	if len(overrides) == 0 {
		return nil
	}
	file, zip, err := s.open(ctx, original)
	if err != nil {
		return fmt.Errorf("读取原始整合包: %w", err)
	}
	defer file.Close()
	manifest, err := modpack.Read(ctx, zip)
	if err != nil {
		return err
	}
	var mappings []archive.Mapping
	for _, mapping := range manifest.Overrides {
		if overrides[mapping.Target] {
			mappings = append(mappings, mapping)
		}
	}
	if len(mappings) != len(overrides) {
		return errors.New("原始整合包未包含所选覆盖文件")
	}
	return zip.Extract(ctx, stage, mappings)
}
