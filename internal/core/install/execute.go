package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

type Progress struct {
	Stage string
	download.Progress
}

type Result struct {
	Plan                  Plan
	Assets                []download.File
	NativeDirectory       string
	LegacyAssetsDirectory string
}

// Execute never persists a success flag. Its caller commits a receipt after
// every stage finishes, and owns the returned native directory from then on.
func Execute(ctx context.Context, client *download.Client, plan Plan, nativeParent string, report func(Progress), gameDirectory ...*os.Root) (Result, error) {
	ctx = withNativeEvidenceBudget(ctx)
	reportPlan(ctx, plan)
	if len(plan.LocalLibraries) > 0 || len(plan.RuntimeLookups) > 0 {
		root, err := os.OpenRoot(client.Root)
		if err != nil {
			return Result{}, err
		}
		err = VerifyLocalLibraries(ctx, root, plan)
		root.Close()
		if err != nil {
			return Result{}, err
		}
	}
	stage := func(name string) func(download.Progress) {
		return func(p download.Progress) {
			if report != nil {
				report(Progress{Stage: name, Progress: p})
			}
		}
	}
	if err := client.Run(ctx, plan.Files, stage("files")); err != nil {
		return Result{}, fmt.Errorf("安装基础文件: %w", err)
	}
	if err := download.Phase(ctx, "读取资源索引"); err != nil {
		return Result{}, err
	}
	root, err := os.OpenRoot(client.Root)
	if err != nil {
		return Result{}, err
	}
	defer root.Close()
	for _, native := range plan.NativeJARs {
		observeNativeArchive(ctx, root, native, nil, "")
	}
	var indexFile *download.File
	for i := range plan.Files {
		if plan.Files[i].Path == plan.AssetIndexPath {
			indexFile = &plan.Files[i]
			break
		}
	}
	if indexFile == nil || indexFile.Size > MaxAssetIndexSize {
		return Result{}, errors.New("asset index missing from plan or exceeds 16MiB")
	}
	f, indexIdentity, err := openAssetFile(root, plan.AssetIndexPath, indexFile.Size)
	if err != nil {
		return Result{}, err
	}
	var encoded strings.Builder
	readErr := streamAsset(ctx, f, &encoded, indexFile.Size, indexFile.SHA1)
	if readErr == nil {
		readErr = sameAssetFile(root, plan.AssetIndexPath, f, indexIdentity)
	}
	f.Close()
	if readErr != nil {
		return Result{}, readErr
	}
	layout, err := ParseAssetPlan([]byte(encoded.String()))
	assets := layout.Files
	if err != nil {
		return Result{}, err
	}
	if err := client.Run(ctx, assets, stage("assets")); err != nil {
		return Result{}, fmt.Errorf("安装资产对象: %w", err)
	}
	if err := download.Phase(ctx, "准备资源与本地库"); err != nil {
		return Result{}, err
	}
	var game *os.Root
	if len(gameDirectory) > 0 {
		game = gameDirectory[0]
	}
	legacy := ""
	if layout.Virtual || layout.MapToResources || plan.NeedsGameAssets {
		id := path.Base(path.Dir(nativeParent))
		legacy, err = PrepareLegacyAssets(ctx, root, game, id, indexFile.SHA1, layout, plan.NeedsGameAssets, stage("legacy-assets"))
		if err != nil {
			return Result{}, fmt.Errorf("准备历史资源: %w", err)
		}
	}
	committed := false
	defer func() {
		if !committed && legacy != "" {
			RemoveLegacyAssets(context.WithoutCancel(ctx), root, path.Base(path.Dir(nativeParent)), legacy)
		}
	}()
	stage("natives")(download.Progress{})
	directory, err := PrepareNatives(ctx, root, nativeParent, plan.Natives)
	if err != nil {
		return Result{}, fmt.Errorf("准备本地库: %w", err)
	}
	if err := ctx.Err(); err != nil {
		root.RemoveAll(directory)
		return Result{}, err
	}
	committed = true
	return Result{Plan: plan, Assets: assets, NativeDirectory: directory, LegacyAssetsDirectory: legacy}, nil
}

// RemoveNativeDirectory is restricted to managed generations under the given
// parent. It never removes the parent or a caller-supplied arbitrary directory.
func RemoveNativeDirectory(root *os.Root, parent, directory string) error {
	if !validNativeGeneration(parent, directory) {
		return errors.New("invalid native generation")
	}
	parentRoot, err := realDirectory(root, parent, false)
	if err != nil {
		return err
	}
	defer parentRoot.Close()
	return parentRoot.RemoveAll(path.Base(directory))
}

func validNativeGeneration(parent, directory string) bool {
	return meta.ValidPath(parent) && meta.ValidPath(directory) && path.Dir(directory) == parent && managedAssetName(path.Base(directory), "ready-")
}

// CleanupNatives removes marked abandoned preparations and unreferenced
// generations under the instance exclusion. Unmarked older generations, unknown
// directories and links are preserved. A receipt may still authorize explicit
// removal of its previous legacy unmarked generation after successful replacement.
func CleanupNatives(root *os.Root, parent, keep string) error {
	if !meta.ValidPath(parent) || (keep != "" && !validNativeGeneration(parent, keep)) {
		return errors.New("invalid native cleanup paths")
	}
	parentRoot, err := realDirectory(root, parent, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer parentRoot.Close()
	retained := ""
	if keep != "" {
		retained = path.Base(keep)
	}
	return cleanupAssetDirectories(context.Background(), parentRoot, parent, "native", retained, ".prepare-", "ready-")
}
