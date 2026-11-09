package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"mcl/internal/core/discovery"
)

type swiftLayout struct{ launcher, selected string }

func swiftSourceLayout(directory string) (*swiftLayout, error) {
	candidates := []swiftLayout{{launcher: directory}}
	if filepath.Base(directory) == "profiles" {
		candidates = append(candidates, swiftLayout{launcher: filepath.Dir(directory)})
	}
	if filepath.Base(filepath.Dir(directory)) == "profiles" {
		candidates = append(candidates, swiftLayout{launcher: filepath.Dir(filepath.Dir(directory)), selected: filepath.Base(directory)})
	}
	for _, candidate := range candidates {
		root, err := openExistingRoot(candidate.launcher)
		if err != nil {
			continue
		}
		valid := true
		for _, name := range []string{"profiles", "meta", "data"} {
			node, err := openRealDirectory(root, name)
			if err != nil {
				valid = false
				break
			}
			if name == "data" {
				info, err := node.Lstat("data.db")
				if err != nil || !info.Mode().IsRegular() {
					valid = false
				}
			}
			node.Close()
			if !valid {
				break
			}
		}
		root.Close()
		if valid {
			return &candidate, nil
		}
	}
	return nil, nil
}

type sqliteSourceFile struct {
	name    string
	info    os.FileInfo
	missing bool
}

// Both base and WAL identities are fixed before reading, then checked again
// after querying. No connection, SHM mapping or source lock file is ever used.
func (s *ExistingGamesStore) scanSwift(ctx context.Context, scope *os.Root, layout swiftLayout) (discovery.Report, error) {
	report := discovery.Report{Root: scope.Name()}
	if s.swiftReader == nil {
		return report, errors.New("Swift实例索引读取器未配置")
	}
	launcher, err := openExistingRoot(layout.launcher)
	if err != nil {
		return report, err
	}
	defer launcher.Close()
	data, err := openRealDirectory(launcher, "data")
	if err != nil {
		return report, err
	}
	defer data.Close()
	if info, err := data.Lstat("data.db-journal"); err == nil && info.Size() > 0 {
		return report, errors.New("Swift实例索引仍有回滚日志，请关闭来源启动器后重试")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return report, err
	}
	files := []sqliteSourceFile{{name: "data.db"}, {name: "data.db-wal"}}
	for index := range files {
		info, err := data.Lstat(files[index].name)
		if index == 1 && errors.Is(err, os.ErrNotExist) {
			files[index].missing = true
			continue
		}
		if err != nil {
			return report, err
		}
		if !info.Mode().IsRegular() || info.Size() > discovery.MaxSQLiteBytes {
			return report, errors.New("Swift实例索引不是普通文件或超过32MiB")
		}
		files[index].info = info
	}
	parts := make([][]byte, 2)
	defer func() {
		for _, part := range parts {
			clear(part)
		}
	}()
	for index, file := range files {
		if file.missing || file.info.Size() == 0 {
			continue
		}
		report.DocumentsRead++
		parts[index], err = readSwiftDocument(ctx, data, file.name, &report.MetadataBytes)
		if err != nil {
			return report, err
		}
	}
	snapshot, err := discovery.SQLiteSnapshot(parts[0], parts[1])
	if err != nil {
		return report, err
	}
	defer clear(snapshot)
	rows, err := s.swiftReader(ctx, snapshot, layout.selected)
	if err != nil {
		return report, err
	}
	for _, file := range files {
		current, err := data.Lstat(file.name)
		if file.missing {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return report, errors.New("Swift实例索引在读取期间发生变化，请重新扫描")
		}
		if err != nil || !current.Mode().IsRegular() || !os.SameFile(current, file.info) || current.Size() != file.info.Size() || !current.ModTime().Equal(file.info.ModTime()) {
			return report, errors.New("Swift实例索引在读取期间发生变化，请重新扫描")
		}
	}
	opened, e := launcher.Stat(".")
	current, e2 := os.Lstat(layout.launcher)
	if e != nil || e2 != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, current) {
		return report, errors.New("Swift启动器目录在读取期间发生变化")
	}
	result, err := discovery.SwiftCandidates(ctx, scope, layout.launcher, rows)
	result.DocumentsRead, result.MetadataBytes = report.DocumentsRead, report.MetadataBytes
	if layout.selected != "" && len(result.Candidates) == 0 && err == nil {
		return result, errors.New("Swift实例索引中没有这个目录的登记，未按文件夹名猜测游戏版本")
	}
	return result, err
}
func (s *ExistingGamesStore) scanRoot(ctx context.Context, root *os.Root) (discovery.Report, error) {
	layout, err := swiftSourceLayout(root.Name())
	if err != nil {
		return discovery.Report{}, err
	}
	if layout != nil {
		return s.scanSwift(ctx, root, *layout)
	}
	return discovery.Scan(ctx, root)
}
