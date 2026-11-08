package loader

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"mcl/internal/core/meta"
)

func verifyProcessorInputs(ctx context.Context, stage *os.Root, known map[string]meta.Download, inputs []string) error {
	for _, name := range inputs {
		fixed, ok := known[name]
		if !ok {
			if err := verifyProcessorDirectory(ctx, stage, known, name); err != nil {
				return fmt.Errorf("处理器目录输入%s: %w", name, err)
			}
			continue
		}
		if _, err := hashForgeFile(ctx, stage, name, fixed.SHA1, fixed.Size); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func verifyProcessorDirectory(ctx context.Context, stage *os.Root, known map[string]meta.Download, name string) error {
	if !ownedProcessorPath(name) {
		return errors.New("目录输入不是已拥有的暂存路径")
	}
	info, err := stage.Lstat(name)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("未知输入不是普通目录")
	}
	directory, err := stage.OpenRoot(name)
	if err != nil {
		return err
	}
	defer directory.Close()
	count := 0
	return fs.WalkDir(directory.FS(), ".", func(relative string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		count++
		if count > 50000 {
			return errors.New("处理器目录输入超过5万项")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("处理器目录输入含链接")
		}
		if entry.IsDir() {
			return nil
		}
		target := path.Join(name, relative)
		fixed, ok := known[target]
		if !ok {
			return errors.New("处理器目录含未固定来源文件")
		}
		_, err := hashForgeFile(ctx, stage, target, fixed.SHA1, fixed.Size)
		return err
	})
}

// Reuse only complete declared outputs matching BOTH previous provenance and
// the processor's expected hashes. Rewrites/in-place steps stay in this task's
// disposable private staging, never in shared inputs or a published generation.
func prepareProcessorOutputs(ctx context.Context, stage *os.Root, known map[string]meta.Download, p preparedProcessor) (bool, error) {
	complete := len(p.outputs) > 0
	for name, expected := range p.outputs {
		info, err := stage.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			complete = false
			continue
		}
		if err != nil {
			return false, err
		}
		if info.IsDir() && p.nativeTask == "MCP_DATA" && expected == "" {
			if err := verifyProcessorDirectory(ctx, stage, known, name); err != nil {
				return false, err
			}
			complete = false
			continue
		}
		previous, ok := known[name]
		if !ok || !info.Mode().IsRegular() {
			return false, errors.New("处理器输出已被未知文件或目录占用")
		}
		if _, err := hashForgeFile(ctx, stage, name, previous.SHA1, previous.Size); err != nil {
			return false, err
		}
		if expected == "" {
			complete = false
		} else if _, err := hashForgeFile(ctx, stage, name, expected, 0); err != nil {
			complete = false
		}
	}
	if complete {
		return true, nil
	}
	for name := range p.outputs {
		if info, err := stage.Lstat(name); err == nil && info.IsDir() && p.nativeTask == "MCP_DATA" {
			if err := stage.RemoveAll(name); err != nil {
				return false, err
			}
			for key := range known {
				if strings.HasPrefix(key, name+"/") {
					delete(known, key)
				}
			}
		}
		if _, exists := known[name]; exists && !slices.Contains(p.inputs, name) && !optionalProcessorOutput(p, name) {
			if err := stage.Remove(name); err != nil {
				return false, err
			}
			delete(known, name)
		}
		if err := stage.MkdirAll(path.Dir(name), 0o700); err != nil {
			return false, err
		}
	}
	return false, ctx.Err()
}
