package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"mcl/internal/core/instance"
	"mcl/internal/core/tasks"
	"mcl/internal/fileio"
	"mcl/internal/platform/instancelock"
)

type DownloadTaskStore struct{ root string }

func NewDownloadTaskStore(root string) *DownloadTaskStore { return &DownloadTaskStore{root} }
func (s *DownloadTaskStore) Lock(ctx context.Context, id string) (io.Closer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return instancelock.AcquireTask(s.root, id)
}
func (s *DownloadTaskStore) open() (*os.Root, error) {
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := root.Mkdir("download-tasks-v1", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	return openRealDirectory(root, "download-tasks-v1")
}
func (s *DownloadTaskStore) List(ctx context.Context) ([]tasks.Group, error) {
	root, err := s.open()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(tasks.MaxGroups + 257)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > tasks.MaxGroups+256 {
		return nil, errors.New("任务目录超过读取边界，请保留原记录并检查")
	}
	var groups []tasks.Group
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !instance.ValidID(id) || entry.Name() != id+".json" {
			continue
		}
		var group tasks.Group
		if err := readTaskJSON(ctx, root, entry.Name(), &group); err != nil {
			return nil, err
		}
		if err := group.Validate(); err != nil {
			return nil, err
		}
		if group.ID != id {
			return nil, errors.New("任务文件与身份不符")
		}
		groups = append(groups, group.Summary())
		if len(groups) > tasks.MaxGroups {
			return nil, errors.New("任务历史超过256项，请先清理已结束任务")
		}
	}
	return groups, ctx.Err()
}
func (s *DownloadTaskStore) Get(ctx context.Context, id string) (tasks.Group, error) {
	var group tasks.Group
	if !instance.ValidID(id) {
		return group, errors.New("任务ID无效")
	}
	root, err := s.open()
	if err != nil {
		return group, err
	}
	defer root.Close()
	if err := readTaskJSON(ctx, root, id+".json", &group); err != nil {
		return group, err
	}
	if err := group.Validate(); err != nil {
		return group, err
	}
	if group.ID != id {
		return group, errors.New("任务文件身份不符")
	}
	return group, nil
}
func (s *DownloadTaskStore) Create(ctx context.Context, group tasks.Group) error {
	guard, err := instancelock.AcquireTaskIndex(s.root)
	if err != nil {
		return err
	}
	defer guard.Close()
	groups, err := s.List(ctx)
	if err != nil {
		return err
	}
	if len(groups) >= tasks.MaxGroups {
		return errors.New("任务历史已满，请先清理已结束任务")
	}
	root, err := s.open()
	if err != nil {
		return err
	}
	defer root.Close()
	if _, err := root.Lstat(group.ID + ".json"); err == nil {
		return errors.New("任务ID已存在")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.Save(ctx, group)
}
func (s *DownloadTaskStore) Save(ctx context.Context, group tasks.Group) error {
	group.Owned = false
	group.Remote = false
	group.Received = 0
	group.Verified = 0
	group.TotalBytes = 0
	group.FileCount = 0
	group.DoneFiles = 0
	if err := group.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(group)
	if err != nil {
		return err
	}
	if len(data) > tasks.MaxDocument {
		return errors.New("任务记录超过48MiB")
	}
	root, err := s.open()
	if err != nil {
		return err
	}
	defer root.Close()
	if err := fileio.WriteAt(ctx, root, group.ID+".json", data, 0600); err != nil {
		return err
	}
	return fileio.SyncDirectory(root)
}
func (s *DownloadTaskStore) Remove(ctx context.Context, id string) error {
	group, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !group.Terminal() {
		return errors.New("只能清理已结束任务，未完成的下载请先取消")
	}
	root, err := s.open()
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(id + ".json"); err != nil {
		return err
	}
	return fileio.SyncDirectory(root)
}

func readTaskJSON(ctx context.Context, root *os.Root, name string, group *tasks.Group) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	before, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() || before.Size() > tasks.MaxDocument {
		return errors.New("任务记录类型或大小无效，已保留原文件")
	}
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return errors.New("任务记录读取时发生变化")
	}
	data, err := io.ReadAll(io.LimitReader(file, tasks.MaxDocument+1))
	if err != nil {
		return err
	}
	after, err := file.Stat()
	if err != nil {
		return err
	}
	if int64(len(data)) != before.Size() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return errors.New("任务记录读取时发生变化")
	}
	// Atomic replacement by another launcher is allowed: this open FD is one
	// complete committed revision. No loose path is reopened for the payload.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(group); err != nil {
		return errors.New("任务记录格式损坏，原文件已保留")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("任务记录包含尾随内容")
	}
	return ctx.Err()
}
