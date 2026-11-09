package download

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
)

// ClearParts only removes private, correctly keyed partial data. It never
// touches verified targets, the user-selected object cache or instance data.
func (c *Client) ClearParts(ctx context.Context) (int64, error) {
	root, err := os.OpenRoot(c.Root)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	if c.Coordination == nil {
		return 0, errors.New("分片维护协调器未配置")
	}
	guard, err := c.Coordination.Parts(ctx, true)
	if err != nil {
		return 0, errors.New("有下载仍在使用分片，暂不能清理")
	}
	defer guard.Close()
	parts, err := realChild(root, partsDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer parts.Close()
	file, err := parts.Open(".")
	if err != nil {
		return 0, err
	}
	entries, err := file.ReadDir(60001)
	file.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	if len(entries) > 60000 {
		return 0, errors.New("分片目录超出维护边界，未清理")
	}
	var total int64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var state partialState
		if err := readPartial(parts, entry.Name(), &state); err != nil {
			return total, err
		}
		expected := File{Path: state.Path, SHA256: state.Object.SHA256, SHA512: state.Object.SHA512}
		expected.SHA1, expected.Size = state.Object.SHA1, state.Object.Size
		if state.Schema != 1 || !SafeTaskPath(state.Path) || FileKey(expected) != state.Key || entry.Name() != state.Key+".json" {
			return total, errors.New("分片所属证据不符，已保留")
		}
		if _, err := state.Object.Key(); err != nil {
			return total, err
		}
		name := state.Key + ".part"
		info, err := parts.Lstat(name)
		if err == nil {
			if !info.Mode().IsRegular() {
				return total, errors.New("分片含非普通文件，已保留")
			}
			if err := parts.Remove(name); err != nil {
				return total, err
			}
			total += info.Size()
		} else if !errors.Is(err, os.ErrNotExist) {
			return total, err
		}
		if err := parts.Remove(entry.Name()); err != nil {
			return total, err
		}
	}
	return total, ctx.Err()
}
