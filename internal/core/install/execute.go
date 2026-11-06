package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"mcl/internal/core/meta"

	"mcl/internal/core/download"
)

type Progress struct {
	Stage string
	download.Progress
}

type Result struct {
	Plan            Plan
	Assets          []download.File
	NativeDirectory string
}

// Execute never persists a success flag. Its caller commits a receipt after
// every stage finishes, and owns the returned native directory from then on.
func Execute(ctx context.Context, client *download.Client, plan Plan, nativeParent string, report func(Progress)) (Result, error) {
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
	root, err := os.OpenRoot(client.Root)
	if err != nil {
		return Result{}, err
	}
	defer root.Close()
	f, err := root.Open(plan.AssetIndexPath)
	if err != nil {
		return Result{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, MaxAssetIndexSize+1))
	f.Close()
	if readErr != nil {
		return Result{}, readErr
	}
	assets, err := PlanAssets(data)
	if err != nil {
		return Result{}, err
	}
	if err := client.Run(ctx, assets, stage("assets")); err != nil {
		return Result{}, fmt.Errorf("安装资产对象: %w", err)
	}
	stage("natives")(download.Progress{})
	directory, err := PrepareNatives(ctx, root, nativeParent, plan.Natives)
	if err != nil {
		return Result{}, fmt.Errorf("准备本地库: %w", err)
	}
	if err := ctx.Err(); err != nil {
		root.RemoveAll(directory)
		return Result{}, err
	}
	return Result{Plan: plan, Assets: assets, NativeDirectory: directory}, nil
}

// RemoveNativeDirectory is restricted to managed generations under the given
// parent. It never removes the parent or a caller-supplied arbitrary directory.
func RemoveNativeDirectory(root *os.Root, parent, directory string) error {
	if !validNativeGeneration(parent, directory) {
		return errors.New("invalid native generation")
	}
	return root.RemoveAll(directory)
}

func validNativeGeneration(parent, directory string) bool {
	return meta.ValidPath(parent) && meta.ValidPath(directory) && path.Dir(directory) == parent && strings.HasPrefix(path.Base(directory), "ready-") && len(path.Base(directory)) == len("ready-")+26
}

// CleanupNatives removes abandoned preparations and unreferenced generations
// while the caller holds the instance exclusion. Other directory names are
// preserved, so recovery cannot delete user files placed beside managed data.
func CleanupNatives(root *os.Root, parent, keep string) error {
	if !meta.ValidPath(parent) || (keep != "" && !validNativeGeneration(parent, keep)) {
		return errors.New("invalid native cleanup paths")
	}
	f, err := root.Open(parent)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		managed := strings.HasPrefix(name, ".prepare-") && len(name) == len(".prepare-")+26 || strings.HasPrefix(name, "ready-") && len(name) == len("ready-")+26
		directory := path.Join(parent, name)
		if managed && directory != keep {
			if err := root.RemoveAll(directory); err != nil {
				return err
			}
		}
	}
	return nil
}
