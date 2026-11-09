package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"mcl/internal/core/market"
)

// UpdateTaskIdentity excludes URLs and presentation; authentication and icon
// changes cannot alter which fixed before/after files a task may recover.
func UpdateTaskIdentity(plan market.UpdatePlan) string {
	type fixed struct {
		Source                       market.Source
		Project, Version, Path, SHA1 string
		Size                         int64
		Enabled                      bool
	}
	fixedFile := func(file market.InstallFile, enabled bool) fixed {
		return fixed{file.Source, file.ProjectID, file.VersionID, file.Path, file.File.SHA1, file.File.Size, enabled}
	}
	type change struct {
		Before     *fixed
		BeforePath string
		After      fixed
	}
	changes := make([]change, 0, len(plan.Changes))
	for _, value := range plan.Changes {
		entry := change{After: fixedFile(value.After, value.Enabled)}
		if value.Before != nil {
			before := fixedFile(value.Before.File, value.Before.Enabled)
			entry.Before = &before
			entry.BeforePath = value.Before.Path
		}
		changes = append(changes, entry)
	}
	data, _ := json.Marshal(struct {
		Request market.UpdateRequest
		Receipt string
		Changes []change
	}{plan.Request, plan.ReceiptSHA256, changes})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (s *AssetUpdateService) recoverTask(ctx context.Context, plan market.UpdatePlan) (AssetUpdateResult, bool, error) {
	if len(plan.Changes) == 0 || len(plan.Changes) > 128 {
		return AssetUpdateResult{}, false, errors.New("固定更新任务数量无效")
	}
	store, ok := s.store.(interface {
		TaskUpdateStatus(context.Context, market.UpdatePlan) (string, bool, error)
	})
	if !ok {
		return AssetUpdateResult{}, false, nil
	}
	lease, err := s.registry.ReserveForRecovery(ctx, plan.Request.InstanceID)
	if err != nil {
		return AssetUpdateResult{}, false, err
	}
	defer lease.Close()
	value, err := s.instances.Get(ctx, plan.Request.InstanceID)
	if err != nil {
		return AssetUpdateResult{}, false, err
	}
	if err := checkTaskInstance(ctx, value); err != nil {
		return AssetUpdateResult{}, false, err
	}
	transaction, applied, err := store.TaskUpdateStatus(ctx, plan)
	if err != nil {
		return AssetUpdateResult{}, false, err
	}
	if applied {
		return AssetUpdateResult{Applied: true}, true, nil
	}
	if transaction == "" {
		return AssetUpdateResult{}, false, nil
	}
	result, err := s.store.RecoverUpdate(ctx, value.ID, transaction)
	if err != nil {
		return result, false, err
	}
	if result.RecoveryRequired {
		return result, false, errors.New("资产更新恢复仍未完成")
	}
	return result, result.Applied, nil
}
