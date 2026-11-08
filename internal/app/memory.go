package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"mcl/internal/core/java"
	"mcl/internal/core/launch"
)

// MemoryStatus estimates physical memory available for applications. Reclaimable
// caches are available, not unconditionally treated as application memory.
type MemoryStatus struct{ TotalBytes, AvailableBytes uint64 }

func (m MemoryStatus) Validate() error {
	if m.TotalBytes == 0 || m.TotalBytes > 1<<60 || m.AvailableBytes > m.TotalBytes {
		return errors.New("系统内存统计无效")
	}
	return nil
}

type MemorySnapshot struct {
	MemoryStatus
	SuggestedMiB int
	CapturedAt   time.Time
}

func (m MemorySnapshot) UsedBytes() uint64 {
	if m.AvailableBytes > m.TotalBytes {
		return 0
	}
	return m.TotalBytes - m.AvailableBytes
}

type MemoryProbe func(context.Context) (MemoryStatus, error)
type MemoryService struct {
	probe MemoryProbe
	arch  string
}

func NewMemoryService(probe MemoryProbe, arch string) *MemoryService {
	return &MemoryService{probe: probe, arch: arch}
}
func (s *MemoryService) Snapshot(ctx context.Context) (MemorySnapshot, error) {
	if err := ctx.Err(); err != nil {
		return MemorySnapshot{}, err
	}
	if s.probe == nil {
		return MemorySnapshot{}, errors.New("系统内存读取尚未配置")
	}
	status, err := s.probe(ctx)
	if err != nil {
		return MemorySnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return MemorySnapshot{}, err
	}
	if err := status.Validate(); err != nil {
		return MemorySnapshot{}, err
	}
	return MemorySnapshot{MemoryStatus: status, SuggestedMiB: launch.AutomaticMemoryMiB(java.Runtime{Arch: s.arch}, status.AvailableBytes), CapturedAt: time.Now().UTC()}, nil
}

// launchMemoryMiB samples afresh during background preparation/handoff. It does
// not use a UI snapshot or force a minimum above the available-memory budget.
func launchMemoryMiB(ctx context.Context, probe MemoryProbe, runtime java.Runtime) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if probe == nil {
		return 0, errors.New("系统内存读取尚未配置")
	}
	status, err := probe(ctx)
	if err != nil {
		return 0, fmt.Errorf("读取当前可用内存失败，请稍后重试: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := status.Validate(); err != nil {
		return 0, err
	}
	limit := launch.AutomaticMemoryMiB(runtime, status.AvailableBytes)
	if limit < 512 {
		return 0, errors.New("当前可用内存不足，保留系统内存后无法分配至少 512 MiB，请关闭其他应用后重试")
	}
	return limit, nil
}
