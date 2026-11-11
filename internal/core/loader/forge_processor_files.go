package loader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"

	"mcl/internal/core/archive"
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
	opened, err := directory.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return errors.New("处理器目录在打开时被替换")
	}
	remaining := make(map[string]bool)
	for key := range known {
		if strings.HasPrefix(key, name+"/") {
			remaining[key] = true
		}
	}
	count := 1
	pending := []string{"."}
	scan := func(relative string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		before, err := directory.Lstat(relative)
		if err != nil {
			return err
		}
		if !before.IsDir() {
			return errors.New("处理器子目录不是普通目录")
		}
		dir, err := directory.Open(relative)
		if err != nil {
			return err
		}
		defer dir.Close()
		opened, err := dir.Stat()
		if err != nil || !opened.IsDir() || !os.SameFile(before, opened) {
			return errors.New("处理器目录输入类型已改变")
		}
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries, readErr := dir.ReadDir(128)
			for _, entry := range entries {
				count++
				if count > 50000 {
					return errors.New("处理器目录输入超过5万项")
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return errors.New("处理器目录输入含链接")
				}
				child := path.Join(relative, entry.Name())
				if !archive.SafePath(child) {
					return errors.New("处理器目录输入含非受管路径")
				}
				if entry.IsDir() {
					pending = append(pending, child)
					continue
				}
				target := path.Join(name, child)
				fixed, ok := known[target]
				if !ok {
					return errors.New("处理器目录含未固定来源文件")
				}
				if _, err := hashForgeFile(ctx, stage, target, fixed.SHA1, fixed.Size); err != nil {
					return err
				}
				delete(remaining, target)
			}
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			if readErr != nil {
				return readErr
			}
		}
	}
	for len(pending) > 0 {
		relative := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if err := scan(relative); err != nil {
			return err
		}
	}
	if len(remaining) != 0 {
		return errors.New("处理器目录缺少已固定的输入成员")
	}
	current, err := stage.Lstat(name)
	if err != nil || !current.IsDir() || !os.SameFile(info, current) {
		return errors.New("处理器目录在检查时被替换")
	}
	return ctx.Err()
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
		previous, ok := known[name]
		if !ok || !info.Mode().IsRegular() {
			return false, errors.New("处理器输出已被未知文件或目录占用")
		}
		if _, err := hashForgeFile(ctx, stage, name, previous.SHA1, previous.Size); err != nil {
			return false, err
		}
		if expected == "" {
			complete = false
		} else if !strings.EqualFold(previous.SHA1, expected) {
			complete = false
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if complete {
		return true, nil
	}
	for name := range p.outputs {
		if err := ctx.Err(); err != nil {
			return false, err
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
