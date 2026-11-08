package install

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
	"strings"

	"mcl/internal/core/download"
	"mcl/internal/core/instance"
	"mcl/internal/core/meta"
	"mcl/internal/fileio"
)

const assetOwnerFile = "owner.json"

type assetOwner struct {
	Schema                             int
	Kind, InstanceID, Nonce, IndexSHA1 string
}

func writeAssetOwner(ctx context.Context, root *os.Root, owner assetOwner) error {
	data, err := json.Marshal(owner)
	if err != nil {
		return err
	}
	f, err := root.OpenFile(assetOwnerFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return ctx.Err()
}
func readAssetOwner(root *os.Root) (assetOwner, error) {
	var owner assetOwner
	info, err := root.Lstat(assetOwnerFile)
	if err != nil {
		return owner, err
	}
	if !info.Mode().IsRegular() || info.Size() > 2048 {
		return owner, errors.New("invalid legacy asset owner")
	}
	f, err := root.Open(assetOwnerFile)
	if err != nil {
		return owner, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 2049))
	if err != nil || len(data) > 2048 {
		return owner, errors.New("legacy asset owner exceeds limit")
	}
	if err := sameAssetFile(root, assetOwnerFile, f, info); err != nil {
		return owner, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&owner); err != nil {
		return owner, err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return owner, errors.New("legacy owner has trailing data")
	}
	if !meta.ValidSHA1(owner.IndexSHA1) {
		return owner, errors.New("invalid legacy index ownership")
	}
	return owner, nil
}
func managedAssetName(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) || len(name) != len(prefix)+26 {
		return false
	}
	return !strings.ContainsFunc(strings.TrimPrefix(name, prefix), func(r rune) bool { return !(r >= 'A' && r <= 'Z' || r >= '2' && r <= '7') })
}

// ValidLegacyAssetsDirectory checks only a confined, managed generation name.
// On-disk ownership is checked separately before destructive cleanup.
func ValidLegacyAssetsDirectory(parent, directory string) bool {
	return meta.ValidPath(parent) && meta.ValidPath(directory) && validLegacyGeneration(parent, directory)
}
func validLegacyGeneration(parent, directory string) bool {
	return path.Dir(directory) == parent && managedAssetName(path.Base(directory), "ready-")
}

// Callers hold the instance OS/session reservation. Cleanup is limited to
// precisely named directories with matching kind, instance and nonce markers.
// Unknown directories and links are preserved, including markerless crash debris.
func cleanupAssetDirectories(ctx context.Context, parent *os.Root, id, kind, keep string, prefixes ...string) error {
	f, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer f.Close()
	for {
		entries, readErr := f.ReadDir(128)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := entry.Name()
			if name == keep || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			nonce := ""
			for _, prefix := range prefixes {
				if managedAssetName(name, prefix) {
					nonce = strings.TrimPrefix(name, prefix)
					break
				}
			}
			if nonce == "" {
				continue
			}
			child, err := realDirectory(parent, name, false)
			if err != nil {
				return err
			}
			identity, statErr := child.Stat(".")
			owner, err := readAssetOwner(child)
			child.Close()
			if statErr != nil {
				return statErr
			}
			if err != nil || owner.Schema != 1 || owner.Kind != kind || owner.InstanceID != id || owner.Nonce != nonce {
				continue
			}
			current, err := parent.Lstat(name)
			if err != nil {
				return err
			}
			if !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(identity, current) {
				return errors.New("legacy directory changed before cleanup")
			}
			if err := parent.RemoveAll(name); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func CleanupLegacyAssets(ctx context.Context, root *os.Root, id, keep string) error {
	if !instance.ValidID(id) {
		return errors.New("invalid legacy asset instance")
	}
	parentName := path.Join("instances", id, "legacy-assets")
	if keep != "" && !validLegacyGeneration(parentName, keep) {
		return errors.New("invalid retained legacy asset generation")
	}
	parent, err := realDirectory(root, parentName, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer parent.Close()
	retained := ""
	if keep != "" {
		retained = path.Base(keep)
	}
	return cleanupAssetDirectories(ctx, parent, id, "virtual", retained, ".prepare-", "ready-")
}
func RemoveLegacyAssets(ctx context.Context, root *os.Root, id, directory string) error {
	if directory == "" {
		return nil
	}
	parent := path.Join("instances", id, "legacy-assets")
	if !instance.ValidID(id) || !validLegacyGeneration(parent, directory) {
		return errors.New("invalid legacy asset generation")
	}
	parentRoot, err := realDirectory(root, parent, false)
	if err != nil {
		return err
	}
	defer parentRoot.Close()
	child, err := realDirectory(parentRoot, path.Base(directory), false)
	if err != nil {
		return err
	}
	identity, statErr := child.Stat(".")
	owner, err := readAssetOwner(child)
	child.Close()
	if statErr != nil {
		return statErr
	}
	if err != nil {
		return err
	}
	if owner.Schema != 1 || owner.Kind != "virtual" || owner.InstanceID != id || owner.Nonce != strings.TrimPrefix(path.Base(directory), "ready-") {
		return errors.New("legacy asset ownership mismatch")
	}
	current, err := parentRoot.Lstat(path.Base(directory))
	if err != nil {
		return err
	}
	if !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(identity, current) {
		return errors.New("legacy asset generation changed before removal")
	}
	return parentRoot.RemoveAll(path.Base(directory))
}

func PrepareLegacyAssets(ctx context.Context, root, game *os.Root, id, indexSHA1 string, plan AssetPlan, needsGameAssets bool, report func(download.Progress)) (directory string, err error) {
	if !instance.ValidID(id) || !meta.ValidSHA1(indexSHA1) {
		return "", errors.New("invalid legacy asset instance or index identity")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateAssetMappings(plan.Mappings); err != nil {
		return "", err
	}
	groups := 0
	if plan.MapToResources {
		groups++
	}
	if plan.Virtual || needsGameAssets {
		groups++
	}
	offset := 0
	progress := func(p download.Progress) {
		if report != nil {
			p.Files += offset
			p.TotalFiles = groups * len(plan.Mappings)
			report(p)
		}
	}
	if plan.MapToResources {
		if game == nil {
			return "", errors.New("历史resources映射缺少已验证的游戏目录")
		}
		if err := prepareResources(ctx, root, game, id, indexSHA1, plan.Mappings, progress); err != nil {
			return "", err
		}
	}
	if plan.MapToResources {
		offset = len(plan.Mappings)
	}
	if !plan.Virtual && !needsGameAssets {
		return "", ctx.Err()
	}
	parentName := path.Join("instances", id, "legacy-assets")
	parent, err := realDirectory(root, parentName, true)
	if err != nil {
		return "", err
	}
	defer parent.Close()
	nonce := rand.Text()
	temporary := ".prepare-" + nonce
	final := "ready-" + nonce
	if err := parent.Mkdir(temporary, 0o700); err != nil {
		return "", err
	}
	stage, err := realDirectory(parent, temporary, false)
	if err != nil {
		return "", err
	}
	defer stage.Close()
	if err := writeAssetOwner(ctx, stage, assetOwner{1, "virtual", id, nonce, indexSHA1}); err != nil {
		return "", err
	}
	published := false
	defer func() {
		if !published {
			stage.Close()
			parent.RemoveAll(temporary)
		}
	}()
	for i, mapping := range plan.Mappings {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		target, err := realDirectory(stage, path.Join("files", path.Dir(mapping.Name)), true)
		if err != nil {
			return "", err
		}
		err = copyAsset(ctx, root, mapping.File.Path, target, path.Base(mapping.Name), mapping.File.Size, mapping.File.SHA1)
		target.Close()
		if err != nil {
			return "", fmt.Errorf("复制历史资源 %s: %w", mapping.Name, err)
		}
		progress(download.Progress{Files: i + 1})
	}
	// Empty indexes still have a valid assets directory.
	files, err := realDirectory(stage, "files", true)
	if err != nil {
		return "", err
	}
	files.Close()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	stage.Close()
	if err := fileio.PublishDirectory(parent, temporary, final); err != nil {
		return "", err
	}
	published = true
	return path.Join(parentName, final), nil
}

// resources is user payload, never a recursively replaceable managed cache.
// Publish only missing files. Equal files are reused; conflicts are retained.
// A canceled/crashed job leaves verified published files for a later retry.
func prepareResources(ctx context.Context, root, game *os.Root, id, indexSHA1 string, mappings []AssetMapping, report func(download.Progress)) error {
	if err := cleanupAssetDirectories(ctx, game, id, "resources-stage", "", ".mcl-resource-stage-"); err != nil {
		return err
	}
	resources, err := realDirectory(game, "resources", true)
	if err != nil {
		return err
	}
	defer resources.Close()
	nonce := rand.Text()
	name := ".mcl-resource-stage-" + nonce
	if err := game.Mkdir(name, 0o700); err != nil {
		return err
	}
	stage, err := realDirectory(game, name, false)
	if err != nil {
		return err
	}
	defer stage.Close()
	if err := writeAssetOwner(ctx, stage, assetOwner{1, "resources-stage", id, nonce, indexSHA1}); err != nil {
		return err
	}
	defer func() { stage.Close(); game.RemoveAll(name) }()
	for i, mapping := range mappings {
		if err := ctx.Err(); err != nil {
			return err
		}
		parent, err := realDirectory(resources, path.Dir(mapping.Name), true)
		if err != nil {
			return err
		}
		file := mapping.File
		existing, statErr := parent.Lstat(path.Base(mapping.Name))
		if statErr == nil {
			if !existing.Mode().IsRegular() {
				parent.Close()
				return fmt.Errorf("历史资源目标不是普通文件: %s", mapping.Name)
			}
			err = verifyAsset(ctx, parent, path.Base(mapping.Name), file.Size, file.SHA1)
		} else if errors.Is(statErr, os.ErrNotExist) {
			err = copyAsset(ctx, root, file.Path, stage, "copy", file.Size, file.SHA1)
			if err == nil {
				err = ctx.Err()
			}
			if err == nil {
				err = fileio.MoveNoReplace(stage, "copy", parent, path.Base(mapping.Name))
			}
			if errors.Is(err, os.ErrExist) {
				err = verifyAsset(ctx, parent, path.Base(mapping.Name), file.Size, file.SHA1)
				stage.Remove("copy")
			}
		} else {
			err = statErr
		}
		parent.Close()
		if err != nil {
			return fmt.Errorf("历史resources冲突或校验失败，现有文件已保留（%s）: %w", mapping.Name, err)
		}
		if report != nil {
			report(download.Progress{Files: i + 1, TotalFiles: len(mappings)})
		}
	}
	return ctx.Err()
}
