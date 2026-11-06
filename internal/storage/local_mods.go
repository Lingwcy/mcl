package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/archive"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
	"mcl/internal/core/mods"
)

type modInspector struct {
	files   map[string]mods.File
	bytes   int64
	modules int
}

func (i *modInspector) inspect(ctx context.Context, file app.AssetFile, reader io.ReaderAt, size int64) {
	if file.Kind != app.AssetMod {
		return
	}
	if i.files == nil {
		i.files = make(map[string]mods.File)
	}
	observation := mods.File{Path: file.Path, Enabled: file.Enabled}
	if i.bytes >= 192<<20 || i.modules >= 8192 {
		observation.Problem = "本次声明检查达到资源上限"
	} else {
		data, err := mods.Read(ctx, reader, size)
		i.bytes += data.ReadBytes
		count := 0
		for _, variant := range data.Variants {
			count += len(variant.Mods)
		}
		if err != nil {
			observation.Problem = err.Error()
		} else if i.modules+count > 8192 {
			observation.Problem = "本次模组声明数量达到上限"
		} else {
			observation.Archive = data
			i.modules += count
		}
	}
	i.files[file.Path] = observation
}

func (i *modInspector) observations(files []app.AssetFile, hashed []app.AssetDigest) []mods.File {
	verified := make(map[string]bool, len(hashed))
	for _, file := range hashed {
		verified[file.File.Path] = true
	}
	var result []mods.File
	for _, file := range files {
		if file.Kind != app.AssetMod {
			continue
		}
		observation, parsed := i.files[file.Path]
		if !verified[file.Path] || !parsed {
			observation = mods.File{Path: file.Path, Enabled: file.Enabled, Problem: "文件未校验，声明未知"}
		}
		result = append(result, observation)
	}
	return result
}

type localModEdit struct {
	before, after string
	enabled       bool
	root          *os.Root // nil reuses the already verified local declaration for a rename.
	source, sha1  string
	size          int64
}

func isLocalMod(name string) bool {
	return path.Dir(name) == "mods" && strings.EqualFold(path.Ext(strings.TrimSuffix(name, ".disabled")), ".jar")
}

func scanLocalModFiles(ctx context.Context, game *os.Root) (app.AssetSnapshot, error) {
	result := app.AssetSnapshot{}
	if err := scanAssets(ctx, game, "mods", app.AssetMod, "", &result); err != nil {
		return result, err
	}
	if result.Truncated {
		return result, errors.New("模组目录超过读取上限，未修改文件")
	}
	slices.SortFunc(result.Files, func(a, b app.AssetFile) int { return strings.Compare(a.Path, b.Path) })
	return result, nil
}

// guardLocalMods runs while the caller holds the instance lease. Every newly
// published file is a private, hash-verified source supplied by that caller.
func (s *AssetStore) guardLocalMods(ctx context.Context, game *os.Root, id, loader string, edits []localModEdit) error {
	edits = slices.DeleteFunc(slices.Clone(edits), func(edit localModEdit) bool { return !isLocalMod(edit.before) && !isLocalMod(edit.after) })
	if len(edits) == 0 {
		return nil
	}
	if len(edits) > 64 {
		return errors.New("单次模组变更超过上限")
	}
	value, err := NewInstanceStore(s.dir).Get(ctx, id)
	if err != nil {
		return err
	}
	runtime := mods.Runtime{GameVersion: value.Version.ID}
	if required, ok := value.RequiredLoader(); ok {
		if loader != "" && loader != required.Type {
			return errors.New("实例加载器与变更计划不一致")
		}
		runtime.Loader, runtime.LoaderVersion = required.Type, required.Version
	} else {
		runtime.Loader = loader
	}
	snapshot, err := scanLocalModFiles(ctx, game)
	if err != nil {
		return err
	}
	inspector := &modInspector{}
	hashed, err := hashAssetsInGame(ctx, game, market.Modrinth, snapshot, nil, func(file app.AssetFile, reader io.ReaderAt, size int64) { inspector.inspect(ctx, file, reader, size) })
	if err != nil {
		return err
	}
	before := inspector.observations(snapshot.Files, hashed.Files)
	removed := make(map[string]bool)
	for _, edit := range edits {
		if edit.before != "" {
			removed[archive.Key(edit.before)] = true
		}
	}
	after := slices.DeleteFunc(slices.Clone(before), func(file mods.File) bool {
		return removed[archive.Key(file.Path)]
	})
	var affected []string
	var staged []struct {
		edit localModEdit
		hash string
	}
	var stagedBytes int64
	for _, edit := range edits {
		if err := ctx.Err(); err != nil {
			return err
		}
		affected = append(affected, edit.before, edit.after)
		var replacement mods.File
		if edit.root != nil {
			stagedBytes += edit.size
			if stagedBytes > 8<<30 {
				return errors.New("单次新增模组超过 8 GiB")
			}
			observed, digest, err := readStagedMod(ctx, edit, inspector)
			if err != nil {
				return err
			}
			replacement = observed
			staged = append(staged, struct {
				edit localModEdit
				hash string
			}{edit, digest})
		} else if edit.after != "" {
			found := false
			for _, file := range before {
				if file.Path == edit.before {
					replacement, found = file, true
					break
				}
			}
			if !found {
				return errors.New("变更文件已不在当前模组快照中")
			}
		}
		if edit.after != "" {
			replacement.Path, replacement.Enabled = edit.after, edit.enabled
			after = append(after, replacement)
		}
	}
	loaders := []string{runtime.Loader}
	if runtime.Loader == "" {
		loaders = nil
		for _, file := range append(slices.Clone(before), after...) {
			if !slices.Contains(affected, file.Path) {
				continue
			}
			for _, variant := range file.Archive.Variants {
				if !slices.Contains(loaders, variant.Loader) {
					loaders = append(loaders, variant.Loader)
				}
			}
		}
		if len(loaders) == 0 {
			loaders = []string{""}
		}
	}
	for _, candidate := range loaders {
		runtime.Loader = candidate
		if err := mods.GuardTransition(ctx, before, after, runtime, affected); err != nil {
			return err
		}
	}
	// Reject changes to the directory or any supporting known file while the
	// declarations were checked. Unverified files remain explicitly unknown.
	current, err := scanLocalModFiles(ctx, game)
	if err != nil {
		return err
	}
	if len(current.Files) != len(snapshot.Files) {
		return errors.New("检查期间模组目录已改变，请重试")
	}
	for index, file := range snapshot.Files {
		other := current.Files[index]
		if file.Path != other.Path || file.Size != other.Size || !file.ModifiedAt.Equal(other.ModifiedAt) {
			return errors.New("检查期间模组文件已改变，请重试")
		}
	}
	for _, file := range hashed.Files {
		if err := verifyLocalAsset(ctx, game, file.File.Path, file.Digest.SHA1, file.Digest.Size); err != nil {
			return fmt.Errorf("检查后的文件已改变，未应用：%s: %w", file.File.Name, err)
		}
	}
	for _, source := range staged {
		if err := verifyLocalAsset(ctx, source.edit.root, source.edit.source, source.hash, source.edit.size); err != nil {
			return fmt.Errorf("待发布的模组已改变: %w", err)
		}
	}
	return ctx.Err()
}

func guardStagedPackMods(ctx context.Context, game *os.Root, value instance.Instance) error {
	loader, configured := value.RequiredLoader()
	if !configured {
		return ctx.Err()
	}
	snapshot, err := scanLocalModFiles(ctx, game)
	if err != nil {
		return err
	}
	inspector := &modInspector{}
	hashes, err := hashAssetsInGame(ctx, game, market.Modrinth, snapshot, nil, func(file app.AssetFile, reader io.ReaderAt, size int64) { inspector.inspect(ctx, file, reader, size) })
	if err != nil {
		return err
	}
	after := inspector.observations(snapshot.Files, hashes.Files)
	runtime := mods.Runtime{GameVersion: value.Version.ID, Loader: loader.Type, LoaderVersion: loader.Version}
	return mods.GuardTransition(ctx, nil, after, runtime, nil)
}

func readStagedMod(ctx context.Context, edit localModEdit, inspector *modInspector) (mods.File, string, error) {
	info, err := edit.root.Lstat(edit.source)
	if err != nil {
		return mods.File{}, "", err
	}
	if !info.Mode().IsRegular() || info.Size() != edit.size {
		return mods.File{}, "", errors.New("待发布模组不是预期普通文件")
	}
	file, err := edit.root.Open(edit.source)
	if err != nil {
		return mods.File{}, "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return mods.File{}, "", errors.New("待发布模组在打开时被替换")
	}
	digest, err := market.Digest(ctx, file, edit.size, false, nil)
	if err != nil {
		return mods.File{}, "", err
	}
	if edit.sha1 != "" && !strings.EqualFold(digest.SHA1, edit.sha1) {
		return mods.File{}, "", errors.New("待发布模组摘要与计划不符")
	}
	inspector.inspect(ctx, app.AssetFile{Path: edit.after, Kind: app.AssetMod, Enabled: edit.enabled}, file, edit.size)
	after, err := file.Stat()
	if err != nil {
		return mods.File{}, "", err
	}
	current, err := edit.root.Lstat(edit.source)
	if err != nil {
		return mods.File{}, "", err
	}
	if !current.Mode().IsRegular() || !os.SameFile(info, current) || !os.SameFile(info, after) || current.Size() != info.Size() || after.Size() != info.Size() || !info.ModTime().Equal(current.ModTime()) || !info.ModTime().Equal(after.ModTime()) {
		return mods.File{}, "", errors.New("待发布模组在检查期间改变")
	}
	return inspector.files[edit.after], digest.SHA1, ctx.Err()
}
