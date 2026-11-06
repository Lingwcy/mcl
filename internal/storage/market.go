package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"mcl/internal/core/archive"
	"mcl/internal/core/download"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
	"mcl/internal/core/meta"
)

type MarketStore struct {
	dir    string
	assets *AssetStore
}

func NewMarketStore(root string) *MarketStore {
	return &MarketStore{dir: root, assets: NewAssetStore(filepath.Join(root, "instances"))}
}

type marketReceipt struct {
	Schema int                  `json:"schema"`
	Files  []market.InstallFile `json:"files"`
}

const maxMarketReceiptSize = 8 << 20
const maxMarketRecords = maxLocalAssets * 2

func (s *MarketStore) Worlds(ctx context.Context, id string) ([]string, error) {
	game, err := s.assets.openGame(ctx, id)
	if err != nil {
		return nil, err
	}
	defer game.Close()
	folder, err := localDirectory(game, "saves")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer folder.Close()
	entries, err := folder.ReadDir(129)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 128 {
		return nil, errors.New("存档数量超过读取上限")
	}
	var worlds []string
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 && market.SafeFilename(entry.Name()) {
			info, err := game.Lstat(path.Join("saves", entry.Name(), "level.dat"))
			if err == nil && info.Mode().IsRegular() {
				worlds = append(worlds, entry.Name())
			}
		}
	}
	return worlds, nil
}
func (s *MarketStore) Check(ctx context.Context, plan market.InstallPlan) error {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	return s.check(ctx, root, plan)
}
func (s *MarketStore) check(ctx context.Context, root *os.Root, plan market.InstallPlan) error {
	if !instance.ValidID(plan.Request.InstanceID) || len(plan.Files) == 0 || len(plan.Files) > 64 {
		return errors.New("无效资产安装计划")
	}
	game, err := s.assets.openGame(ctx, plan.Request.InstanceID)
	if err != nil {
		return err
	}
	defer game.Close()
	receipt, err := readMarketReceipt(ctx, root, plan.Request.InstanceID)
	if err != nil {
		return err
	}
	paths := make(map[string]bool)
	var total int64
	for _, f := range plan.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if f.Source != plan.Request.Source || !market.ValidID(f.ProjectID) || !market.ValidID(f.VersionID) ||
			!meta.ValidPath(f.Path) || !market.SafeFilename(f.File.Filename) || path.Base(f.Path) != f.File.Filename ||
			!meta.ValidSHA1(f.File.SHA1) || (f.File.URL != "" && !market.SafeURL(f.File.URL)) ||
			f.File.Size <= 0 || f.File.Size > download.MaxFileSize {
			return errors.New("资产文件计划无效")
		}
		if err := f.Metadata.Validate(); err != nil {
			return err
		}
		key := archive.Key(f.Path)
		if paths[key] {
			return errors.New("安装计划包含重复或别名路径")
		}
		paths[key] = true
		total += f.File.Size
		if total > 8<<30 {
			return errors.New("单次资产安装超过 8 GiB")
		}
		directory := ""
		extension := ".zip"
		switch plan.Request.Kind {
		case market.Mod:
			directory = "mods"
			extension = ".jar"
		case market.Shader:
			directory = "shaderpacks"
		case market.ResourcePack:
			directory = "resourcepacks"
		case market.DataPack:
			if !market.SafeFilename(plan.Request.World) {
				return errors.New("存档路径无效")
			}
			directory = path.Join("saves", plan.Request.World, "datapacks")
			if info, err := game.Lstat(path.Join("saves", plan.Request.World, "level.dat")); err != nil || !info.Mode().IsRegular() {
				return errors.New("目标存档不存在")
			}
		default:
			return errors.New("不支持此安装计划")
		}
		if path.Dir(f.Path) != directory || !strings.EqualFold(path.Ext(f.Path), extension) {
			return errors.New("资产目标目录或扩展名与类型不匹配")
		}
		// Existing symlink components are refused; Root also confines races.
		current := ""
		for _, part := range strings.Split(directory, "/") {
			current = path.Join(current, part)
			info, err := game.Lstat(current)
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			if err != nil {
				return err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("资产目录不是普通目录")
			}
		}
		if info, err := game.Lstat(f.Path); err == nil {
			if !info.Mode().IsRegular() {
				return errors.New("同名目标不是普通文件")
			}
			if err := verifyLocalAsset(ctx, game, f.Path, f.File.SHA1, f.File.Size); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("%s 已存在不同文件，保留原文件", f.File.Filename)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if _, err := game.Lstat(f.Path + ".disabled"); err == nil {
			return fmt.Errorf("%s 已有禁用文件，先管理该文件后重试", f.File.Filename)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		for _, old := range receipt.Files {
			if old.Source == f.Source && old.ProjectID == f.ProjectID && old.Path != f.Path && strings.HasPrefix(old.Path, directory+"/") {
				for _, candidate := range []string{old.Path, old.Path + ".disabled"} {
					if _, err := game.Lstat(candidate); err == nil {
						return errors.New("此项目已有其他版本（包括停用文件），请先管理已有文件后重试")
					} else if !errors.Is(err, os.ErrNotExist) {
						return err
					}
				}
			}
		}
	}
	return guardPlannedConflicts(ctx, game, receipt, plan)
}
func (s *MarketStore) Commit(ctx context.Context, plan market.InstallPlan) (err error) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := s.check(ctx, root, plan); err != nil {
		return err
	}
	game, err := s.assets.openGame(ctx, plan.Request.InstanceID)
	if err != nil {
		return err
	}
	defer game.Close()
	type stagedFile struct {
		file      market.InstallFile
		temp      string
		published bool
	}
	var staged []stagedFile
	committed := false
	defer func() {
		// Ignore task cancellation while reclaiming our own publications. A
		// replaced or modified target is kept with an explicit error.
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer stop()
		for _, entry := range staged {
			if !committed && entry.published {
				if rollbackErr := removeMarketPublication(cleanup, game, entry.temp, entry.file); rollbackErr != nil {
					err = errors.Join(err, rollbackErr)
				}
			}
			game.Remove(entry.temp)
		}
	}()
	var edits []localModEdit
	var reused []market.InstallFile
	for _, f := range plan.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := game.Lstat(f.Path); err == nil {
			reused = append(reused, f)
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := game.MkdirAll(path.Dir(f.Path), 0o700); err != nil {
			return err
		}
		temp := path.Join(path.Dir(f.Path), ".mcl-"+rand.Text()+".tmp")
		if err := copyVerifiedCache(ctx, root, game, temp, f.File.SHA1, "", f.File.Size); err != nil {
			return err
		}
		staged = append(staged, stagedFile{file: f, temp: temp})
		if plan.Request.Kind == market.Mod {
			edits = append(edits, localModEdit{after: f.Path, enabled: true, root: game, source: temp, size: f.File.Size, sha1: f.File.SHA1})
		}
	}
	if err := s.assets.guardLocalMods(ctx, game, plan.Request.InstanceID, plan.Request.Loader, edits); err != nil {
		return err
	}
	// Recheck catalog conflicts and reused targets after staging and local
	// inspection, before any new asset becomes visible.
	if err := s.check(ctx, root, plan); err != nil {
		return err
	}
	for _, file := range reused {
		if err := verifyLocalAsset(ctx, game, file.Path, file.File.SHA1, file.File.Size); err != nil {
			return fmt.Errorf("复用文件已改变 %s: %w", file.File.Filename, err)
		}
	}
	for index := range staged {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry := &staged[index]
		if err := game.Link(entry.temp, entry.file.Path); err != nil {
			return fmt.Errorf("写入 %s 失败，已有文件已保留: %w", entry.file.File.Filename, err)
		}
		entry.published = true
	}
	receipt, err := readMarketReceipt(ctx, root, plan.Request.InstanceID)
	if err != nil {
		return err
	}
	for _, f := range plan.Files {
		receipt.Files = slices.DeleteFunc(receipt.Files, func(old market.InstallFile) bool {
			return old.Path == f.Path && (old.Source == f.Source || !strings.EqualFold(old.File.SHA1, f.File.SHA1))
		})
		receipt.Files = append(receipt.Files, f)
	}
	if err := writeMarketReceiptAt(ctx, root, path.Join("instances", plan.Request.InstanceID, "market.json"), receipt); err != nil {
		return err
	}
	committed = true
	return nil
}
func removeMarketPublication(ctx context.Context, game *os.Root, temp string, file market.InstallFile) error {
	current, err := game.Lstat(file.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	private, err := game.Lstat(temp)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(private, current) {
		return fmt.Errorf("%s 已被替换，已保留现有文件", file.File.Filename)
	}
	if err := verifyLocalAsset(ctx, game, file.Path, file.File.SHA1, file.File.Size); err != nil {
		return fmt.Errorf("%s 无法确认可回滚，已保留: %w", file.File.Filename, err)
	}
	current, err = game.Lstat(file.Path)
	if err != nil {
		return err
	}
	if !os.SameFile(private, current) || !private.ModTime().Equal(current.ModTime()) {
		return fmt.Errorf("%s 在回滚期间改变，已保留", file.File.Filename)
	}
	return game.Remove(file.Path)
}
func readMarketReceipt(ctx context.Context, root *os.Root, id string) (marketReceipt, error) {
	return readMarketReceiptAt(ctx, root, path.Join("instances", id, "market.json"))
}

func readMarketReceiptAt(ctx context.Context, root *os.Root, name string) (marketReceipt, error) {
	result := marketReceipt{Schema: 1}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	file, err := root.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxMarketReceiptSize+1))
	if err != nil {
		return result, err
	}
	if len(data) > maxMarketReceiptSize {
		return result, errors.New("资产记录过大")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&result); err != nil {
		return result, errors.New("资产记录损坏，原文件已保留")
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) || result.Schema != 1 {
		return result, errors.New("资产记录格式不支持，原文件已保留")
	}
	if len(result.Files) > maxMarketRecords {
		return result, errors.New("资产记录条目超出限制")
	}
	seen := make(map[string]bool, len(result.Files))
	for _, file := range result.Files {
		if !meta.ValidPath(file.Path) || !market.ValidID(file.ProjectID) || !market.ValidID(file.VersionID) || !meta.ValidSHA1(file.File.SHA1) || (file.Source != market.Modrinth && file.Source != market.CurseForge) {
			return result, errors.New("资产记录条目无效，原文件已保留")
		}
		key := string(file.Source) + ":" + file.Path
		if seen[key] {
			return result, errors.New("资产来源记录重复，原文件已保留")
		}
		seen[key] = true
		if err := file.Metadata.Validate(); err != nil {
			return result, err
		}
	}
	return result, nil
}
