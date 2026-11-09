package app

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Paths are inspection results, never caller-selected deletion targets. The
// store recomputes and validates them against the authoritative instance.
type InstanceDeletionPlan struct {
	InstanceID, Name, Fingerprint           string
	MetadataPath, GamePath, ExternalPayload string
	Pending, PayloadGone                    bool
}
type InstanceDeletionRepository interface {
	Lock(context.Context, string) (io.Closer, error)
	Inspect(context.Context, string) (InstanceDeletionPlan, error)
	Mark(context.Context, InstanceDeletionPlan) error
	PayloadRemoved(context.Context, InstanceDeletionPlan) error
}
type InstanceDeletionRegistry interface {
	ReserveForDeletion(context.Context, string) (SessionLease, error)
}
type InstanceDeletionService struct {
	store    InstanceDeletionRepository
	registry InstanceDeletionRegistry
	trash    func(string) error
}

func NewInstanceDeletionService(store InstanceDeletionRepository, registry InstanceDeletionRegistry, trash func(string) error) *InstanceDeletionService {
	return &InstanceDeletionService{store: store, registry: registry, trash: trash}
}
func (s *InstanceDeletionService) Inspect(ctx context.Context, id string) (InstanceDeletionPlan, error) {
	lock, err := s.store.Lock(ctx, id)
	if err != nil {
		return InstanceDeletionPlan{}, err
	}
	defer lock.Close()
	lease, err := s.registry.ReserveForDeletion(ctx, id)
	if err != nil {
		return InstanceDeletionPlan{}, err
	}
	defer lease.Close()
	return s.store.Inspect(ctx, id)
}
func (s *InstanceDeletionService) Delete(ctx context.Context, expected InstanceDeletionPlan) error {
	if s.trash == nil {
		return errors.New("系统废纸篓服务未配置")
	}
	guard, err := s.store.Lock(ctx, expected.InstanceID)
	if err != nil {
		return err
	}
	defer guard.Close()
	lease, err := s.registry.ReserveForDeletion(ctx, expected.InstanceID)
	if err != nil {
		return err
	}
	defer lease.Close()
	current, err := s.store.Inspect(ctx, expected.InstanceID)
	if err != nil {
		return err
	}
	if current.Fingerprint != expected.Fingerprint || current.MetadataPath != expected.MetadataPath || current.GamePath != expected.GamePath || current.ExternalPayload != expected.ExternalPayload {
		return errors.New("实例或删除范围已变化，请重新确认")
	}
	if err := s.store.Mark(ctx, current); err != nil {
		return err
	}
	if current.ExternalPayload != "" && !current.PayloadGone {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.trash(current.ExternalPayload); err != nil {
			return fmt.Errorf("实例删除未完成，游戏数据未移入废纸篓，可重试: %w", err)
		}
		// Once the native operation returns, persist its outcome even if the
		// preparation context was canceled while the OS trash operation ran.
		if err := s.store.PayloadRemoved(context.WithoutCancel(ctx), current); err != nil {
			return fmt.Errorf("游戏数据已移入废纸篓，实例记录尚未删除，可重试: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("删除已中断，剩余实例记录可重试清理: %w", err)
	}
	// Windows needs the central lock's file handle closed before moving its
	// containing directory. The persistent marker rejects all normal operations;
	// the separate deletion guard remains locked across this final OS call.
	if err := lease.Close(); err != nil {
		return err
	}
	if err := s.trash(current.MetadataPath); err != nil {
		return fmt.Errorf("实例记录尚未移入废纸篓，请重新执行删除以完成清理: %w", err)
	}
	return nil
}
