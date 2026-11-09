// Package tasks models durable operations without UI or native dependencies.
package tasks

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/download"
	"mcl/internal/core/instance"
)

const MaxGroups = 256
const MaxFiles = 110000
const MaxDocument = 48 << 20

var ErrPaused = errors.New("任务已暂停")
var ErrCanceled = fmt.Errorf("任务已取消: %w", context.Canceled)

type Intent struct {
	Kind, Name, InstanceID, Fingerprint string
	Payload                             json.RawMessage
}
type File struct {
	Key, Path, State          string
	Received, Verified, Total int64
}
type Group struct {
	Evidence                       diagnostics.Snapshot `json:",omitempty"`
	Identities                     map[string]string
	Received, Verified, TotalBytes int64
	FileCount, DoneFiles           int
	Schema                         int
	ID                             string
	Intent                         Intent
	State, Stage, Reason           string
	CreatedAt, UpdatedAt           time.Time
	Attempt                        uint64
	Files                          []File
	Reused                         int
	Owned                          bool `json:"-"`
	Remote                         bool `json:"-"`
}

func (g Group) Validate() error {
	if err := g.Evidence.Validate(); err != nil {
		return err
	}
	if (g.Schema != 1 && g.Schema != 2) || !instance.ValidID(g.ID) || g.CreatedAt.IsZero() || g.UpdatedAt.IsZero() || !utf8.ValidString(g.Intent.Name) || len(g.Intent.Name) > 320 || strings.ContainsFunc(g.Intent.Name, unicode.IsControl) || g.Intent.InstanceID != "" && !instance.ValidID(g.Intent.InstanceID) || len(g.Intent.Payload) > 4<<20 || !json.Valid(g.Intent.Payload) || len(g.Files) > MaxFiles {
		return errors.New("下载任务记录身份或边界无效")
	}
	switch g.Intent.Kind {
	case "game", "java", "market", "pack", "update", "preparation", "files", "archive", "repair":
	default:
		return errors.New("下载任务种类无效")
	}
	switch g.State {
	case "queued", "running", "pausing", "paused", "canceling", "interrupted", "failed", "completed", "canceled":
	default:
		return errors.New("下载任务状态无效")
	}
	if len(g.Stage) > 160 || len(g.Reason) > 320 {
		return errors.New("下载任务状态字段过长")
	}
	if len(g.Identities) > 256 {
		return errors.New("任务来源身份记录越界")
	}
	for key, value := range g.Identities {
		if len(key) > 256 || len(value) != 64 {
			return errors.New("任务来源身份无效")
		}
		if _, err := hex.DecodeString(value); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, file := range g.Files {
		if len(file.Key) != 64 || seen[file.Key] || !download.SafeTaskPath(file.Path) || file.Total < 0 || file.Total > download.MaxFileSize || file.Received < 0 || file.Received > file.Total || file.Verified < 0 || file.Verified > file.Total {
			return errors.New("下载任务文件条目无效")
		}
		if _, err := hex.DecodeString(file.Key); err != nil {
			return errors.New("任务文件键格式无效")
		}
		switch file.State {
		case "skipped", "pending", "checking", "downloading", "verifying", "publishing", "done", "paused", "failed", "canceled", "interrupted", "completed", "queued", "running", "pausing", "canceling":
		default:
			return errors.New("任务文件状态无效")
		}
		seen[file.Key] = true
	}
	return nil
}
func (g Group) Terminal() bool {
	return g.State == "completed" || g.State == "canceled" || g.State == "failed"
}

func (g Group) Summary() Group {
	g.Evidence = g.Evidence.Clone()
	g.Received, g.Verified, g.TotalBytes = 0, 0, 0
	g.FileCount = 0
	g.DoneFiles = 0
	for _, f := range g.Files {
		if f.State == "skipped" {
			continue
		}
		g.FileCount++
		g.Received += f.Received
		g.Verified += f.Verified
		g.TotalBytes += f.Total
		if f.State == "done" {
			g.DoneFiles++
		}
	}
	g.Identities = nil
	g.Files = nil
	g.Intent.Payload = nil
	return g
}
