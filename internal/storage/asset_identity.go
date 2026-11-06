package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/archive"
	"mcl/internal/core/download"
	"mcl/internal/core/market"
	"mcl/internal/core/meta"
	"mcl/internal/fileio"
)

func (s *AssetStore) HashAssets(ctx context.Context, id string, source market.Source, report func(download.Progress)) (app.AssetHashSnapshot, error) {
	snapshot, err := s.List(ctx, id)
	if err != nil {
		return app.AssetHashSnapshot{}, err
	}
	if snapshot.Warning != "" {
		return app.AssetHashSnapshot{}, errors.New(snapshot.Warning)
	}
	return s.hashAssets(ctx, id, source, snapshot, report)
}

func (s *AssetStore) HashAsset(ctx context.Context, id string, file app.AssetFile, fingerprint bool) (app.AssetDigest, error) {
	if file.Directory || file.Size <= 0 || file.Size > download.MaxFileSize || !assetPath(file.Path) {
		return app.AssetDigest{}, errors.New("仅支持关联实例中的 JAR / ZIP 文件")
	}
	parent := path.Dir(file.Path)
	var kind app.AssetKind
	switch parent {
	case "mods":
		kind = app.AssetMod
	case "resourcepacks":
		kind = app.AssetResourcePack
	case "shaderpacks":
		kind = app.AssetShader
	default:
		kind = app.AssetDataPack // assetPath already checked the world path.
	}
	if file.Kind != kind {
		return app.AssetDigest{}, errors.New("本地资产分类与路径不符")
	}
	game, err := s.openGame(ctx, id)
	if err != nil {
		return app.AssetDigest{}, err
	}
	defer game.Close()
	names, err := assetIdentityNames(game, parent)
	if err != nil {
		return app.AssetDigest{}, err
	}
	if names[identityName(path.Base(file.Path))] != 1 {
		return app.AssetDigest{}, errors.New("本地资产缺失或存在同名文件，请刷新实例")
	}
	digest, err := digestAsset(ctx, game, file, fingerprint, nil)
	file.Origin, file.Origins = nil, nil
	return app.AssetDigest{File: file, Digest: digest}, err
}

func identityName(name string) string {
	return strings.TrimSuffix(archive.Key(name), ".disabled")
}

func assetIdentityNames(game *os.Root, parent string) (map[string]int, error) {
	folder, err := localDirectory(game, parent)
	if err != nil {
		return nil, err
	}
	defer folder.Close()
	names, err := folder.Readdirnames(maxLocalAssets + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(names) > maxLocalAssets {
		return nil, errors.New("资产目录超过读取上限")
	}
	counts := make(map[string]int, len(names))
	for _, name := range names {
		counts[identityName(name)]++
	}
	return counts, nil
}

func (s *AssetStore) hashAssets(ctx context.Context, id string, source market.Source, snapshot app.AssetSnapshot, report func(download.Progress)) (app.AssetHashSnapshot, error) {
	return s.hashAssetsWithInspector(ctx, id, source, snapshot, report, nil)
}

func (s *AssetStore) hashAssetsWithInspector(ctx context.Context, id string, source market.Source, snapshot app.AssetSnapshot, report func(download.Progress), inspect func(app.AssetFile, io.ReaderAt, int64)) (app.AssetHashSnapshot, error) {
	game, err := s.openGame(ctx, id)
	if err != nil {
		return app.AssetHashSnapshot{}, err
	}
	defer game.Close()
	return hashAssetsInGame(ctx, game, source, snapshot, report, inspect)
}

func hashAssetsInGame(ctx context.Context, game *os.Root, source market.Source, snapshot app.AssetSnapshot, report func(download.Progress), inspect func(app.AssetFile, io.ReaderAt, int64)) (app.AssetHashSnapshot, error) {
	result := app.AssetHashSnapshot{Truncated: snapshot.Truncated}
	counts := make(map[string]int)
	for _, file := range snapshot.Files {
		counts[archive.Key(strings.TrimSuffix(file.Path, ".disabled"))]++
	}
	var files []app.AssetFile
	var total int64
	for _, file := range snapshot.Files {
		if file.Directory || !assetPath(file.Path) || file.Size <= 0 || file.Size > download.MaxFileSize || counts[archive.Key(strings.TrimSuffix(file.Path, ".disabled"))] > 1 || total+file.Size > 8<<30 {
			result.Skipped++
			continue
		}
		files = append(files, file)
		total += file.Size
	}
	var read int64
	multiplier := int64(1)
	if source == market.CurseForge {
		multiplier = 2
	}
	totalWork := total * multiplier
	for index, file := range files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		var currentRead int64
		digest, err := digestAssetInspect(ctx, game, file, source == market.CurseForge, func(bytes int64) {
			currentRead = bytes
			if report != nil {
				report(download.Progress{Files: index, TotalFiles: len(files), Bytes: read + bytes, TotalBytes: totalWork})
			}
		}, func(reader io.ReaderAt, size int64) {
			if inspect != nil {
				inspect(file, reader, size)
			}
		})
		if err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			result.Skipped++
			if len(result.Warnings) < 8 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %s", file.Name, err))
			}
			totalWork -= file.Size*multiplier - currentRead
			read += currentRead
			continue
		}
		file.Origin = nil
		file.Origins = nil
		result.Files = append(result.Files, app.AssetDigest{File: file, Digest: digest})
		read += file.Size * multiplier
		if report != nil {
			report(download.Progress{Files: index + 1, TotalFiles: len(files), Bytes: read, TotalBytes: totalWork})
		}
	}
	return result, nil
}

// digestAsset checks the same opened regular file before and after hashing,
// including its current path entry. It does not require a download URL.
func digestAsset(ctx context.Context, root *os.Root, expected app.AssetFile, withFingerprint bool, progress func(int64)) (market.FileDigest, error) {
	return digestAssetInspect(ctx, root, expected, withFingerprint, progress, nil)
}

func digestAssetInspect(ctx context.Context, root *os.Root, expected app.AssetFile, withFingerprint bool, progress func(int64), inspect func(io.ReaderAt, int64)) (market.FileDigest, error) {
	if !assetPath(expected.Path) {
		return market.FileDigest{}, errors.New("资产路径无效")
	}
	directory, err := localDirectory(root, path.Dir(expected.Path))
	if err != nil {
		return market.FileDigest{}, err
	}
	directory.Close()
	before, err := root.Lstat(expected.Path)
	if err != nil {
		return market.FileDigest{}, err
	}
	if !before.Mode().IsRegular() || before.Size() != expected.Size || (!expected.ModifiedAt.IsZero() && !before.ModTime().Equal(expected.ModifiedAt)) {
		return market.FileDigest{}, errors.New("文件已改变，请刷新后重试")
	}
	file, err := root.Open(expected.Path)
	if err != nil {
		return market.FileDigest{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return market.FileDigest{}, err
	}
	if !os.SameFile(before, opened) {
		return market.FileDigest{}, errors.New("文件在打开时被替换")
	}
	digest, err := market.Digest(ctx, file, expected.Size, withFingerprint, progress)
	if err != nil {
		return market.FileDigest{}, err
	}
	if inspect != nil {
		inspect(file, expected.Size)
	}
	if err := ctx.Err(); err != nil {
		return market.FileDigest{}, err
	}
	after, err := file.Stat()
	if err != nil {
		return market.FileDigest{}, err
	}
	current, err := root.Lstat(expected.Path)
	if err != nil {
		return market.FileDigest{}, err
	}
	if !current.Mode().IsRegular() || !os.SameFile(before, current) || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || current.Size() != before.Size() || !current.ModTime().Equal(before.ModTime()) {
		return market.FileDigest{}, errors.New("文件在识别期间发生变化")
	}
	return digest, nil
}

func verifyLocalAsset(ctx context.Context, root *os.Root, name, sha1 string, size int64) error {
	if !meta.ValidSHA1(sha1) {
		return errors.New("资产校验摘要无效")
	}
	before, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() || before.Size() != size {
		return errors.New("资产类型或大小不匹配")
	}
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, opened) {
		return errors.New("资产文件被替换")
	}
	digest, err := market.Digest(ctx, file, size, false, nil)
	if err != nil {
		return err
	}
	current, err := root.Lstat(name)
	if err != nil {
		return err
	}
	after, err := file.Stat()
	if err != nil {
		return err
	}
	if !strings.EqualFold(digest.SHA1, sha1) || !current.Mode().IsRegular() || !os.SameFile(before, current) || !os.SameFile(before, after) || current.Size() != size || after.Size() != size || !after.ModTime().Equal(before.ModTime()) || !current.ModTime().Equal(before.ModTime()) {
		return errors.New("资产摘要或文件快照不匹配")
	}
	return nil
}

func (s *AssetStore) RecordIdentities(ctx context.Context, id string, source market.Source, observations []app.AssetIdentification) (int, error) {
	if len(observations) == 0 || len(observations) > 100 || (source != market.Modrinth && source != market.CurseForge) {
		return 0, errors.New("无效来源记录批次")
	}
	game, err := s.openGame(ctx, id)
	if err != nil {
		return 0, err
	}
	defer game.Close()
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	receipt, err := readMarketReceiptAt(ctx, root, path.Join(id, "market.json"))
	if err != nil {
		return 0, err
	}
	matched := 0
	for _, observation := range observations {
		asset := observation.Asset
		if !assetPath(asset.File.Path) || !meta.ValidSHA1(asset.Digest.SHA1) || asset.File.Size != asset.Digest.Size {
			return 0, errors.New("来源记录快照无效")
		}
		if observation.RequireUniquePath {
			names, err := assetIdentityNames(game, path.Dir(asset.File.Path))
			if err != nil {
				return 0, err
			}
			if names[identityName(path.Base(asset.File.Path))] != 1 {
				return 0, errors.New("资产目录出现同名文件或文件缺失，请刷新实例")
			}
		}
		current, err := digestAsset(ctx, game, asset.File, false, nil)
		if err != nil {
			return 0, err
		}
		if current.SHA1 != asset.Digest.SHA1 {
			return 0, errors.New("联网识别期间文件已改变，未保存本批来源")
		}
		canonical := strings.TrimSuffix(asset.File.Path, ".disabled")
		key := archive.Key(canonical)
		origin := observation.Origin
		if origin != nil {
			if origin.Source != source || archive.Key(origin.Path) != key || !market.ValidID(origin.ProjectID) || !market.ValidID(origin.VersionID) || !strings.EqualFold(origin.File.SHA1, current.SHA1) || origin.File.Size != current.Size {
				return 0, errors.New("来源记录与实际文件不匹配")
			}
			if err := origin.Metadata.Validate(); err != nil {
				return 0, err
			}
		}
		// A newly observed digest disproves stale identities from either source.
		// A successful negative lookup does not discard a prior identical hash.
		receipt.Files = slices.DeleteFunc(receipt.Files, func(old market.InstallFile) bool {
			return archive.Key(old.Path) == key && (!strings.EqualFold(old.File.SHA1, current.SHA1) || (origin != nil && old.Source == source))
		})
		if origin != nil {
			receipt.Files = append(receipt.Files, *origin)
			matched++
		}
	}
	if err := writeMarketReceiptAt(ctx, root, path.Join(id, "market.json"), receipt); err != nil {
		return 0, err
	}
	return matched, nil
}

func writeMarketReceiptAt(ctx context.Context, root *os.Root, name string, receipt marketReceipt) error {
	if receipt.Schema != 1 || len(receipt.Files) > maxMarketRecords {
		return errors.New("资产记录数量超出限制")
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	if len(data) > maxMarketReceiptSize {
		return errors.New("资产记录超出大小上限")
	}
	return fileio.WriteAt(ctx, root, name, data, 0o600)
}
