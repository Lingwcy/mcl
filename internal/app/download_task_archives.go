package app

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"mcl/internal/core/instance"
	"mcl/internal/core/market"
)

type archiveTask struct {
	Request   ArchiveImportRequest
	Archive   market.File
	Directory string
}
type repairTask struct {
	Plan  PackRepairPlan
	Paths []string
}

func (s *ArchiveImporter) checkArchiveTask(ctx context.Context, file market.File) error {
	scope := taskScope(ctx)
	if scope == nil {
		return ctx.Err()
	}
	scope.execution.mu.Lock()
	intent := scope.execution.group.Intent
	scope.execution.mu.Unlock()
	if intent.Kind != "archive" {
		return ctx.Err()
	}
	var saved archiveTask
	if err := json.Unmarshal(intent.Payload, &saved); err != nil {
		return err
	}
	if file.SHA1 != saved.Archive.SHA1 || file.Size != saved.Archive.Size || file.Filename != saved.Archive.Filename {
		return errors.New("整合包或地图固定版本文件已变化，请重新选择")
	}
	return ctx.Err()
}
func (s *ArchiveImporter) repairTaskPlan(ctx context.Context, old PackRepairPlan, paths []string) (PackRepairPlan, []string, error) {
	current, err := s.InspectRepair(ctx, old.InstanceID)
	if err != nil {
		return current, nil, err
	}
	if current.Receipt != old.Receipt {
		return current, nil, errors.New("整合包来源记录已改变，请重新检查")
	}
	var selected []string
	var existing []PackRepairEntry
	for _, name := range paths {
		previous, ok := findRepairEntry(old.Entries, name)
		if !ok {
			return current, nil, errors.New("固定修复清单缺少所选路径")
		}
		actual, pending := findRepairEntry(current.Entries, name)
		if !pending {
			existing = append(existing, previous)
			continue
		}
		if previous.Override != actual.Override || previous.File.SHA1 != actual.File.SHA1 || previous.File.SHA512 != actual.File.SHA512 || previous.File.Size != actual.File.Size || previous.File.ProjectID != actual.File.ProjectID || previous.File.VersionID != actual.File.VersionID {
			return current, nil, errors.New("固定修复文件已变化，请重新检查")
		}
		selected = append(selected, name)
	}
	if len(existing) > 0 {
		checker, ok := s.store.(interface {
			CheckTaskRepairEntries(context.Context, instance.Instance, PackRepairPlan, []PackRepairEntry) error
		})
		if !ok {
			return current, nil, errors.New("修复任务结果校验未配置")
		}
		lease, err := s.registry.Reserve(ctx, old.InstanceID)
		if err != nil {
			return current, nil, err
		}
		defer lease.Close()
		value, err := s.instances.Get(ctx, old.InstanceID)
		if err != nil {
			return current, nil, err
		}
		if err := checkTaskInstance(ctx, value); err != nil {
			return current, nil, err
		}
		if err := checker.CheckTaskRepairEntries(ctx, value, old, existing); err != nil {
			return current, nil, err
		}
	}
	return current, selected, nil
}
func findRepairEntry(entries []PackRepairEntry, name string) (PackRepairEntry, bool) {
	for _, entry := range entries {
		if entry.Path == name {
			return entry, true
		}
	}
	return PackRepairEntry{}, false
}
func (s *ArchiveImporter) taskRestore(ctx context.Context, plan PackRepairPlan, paths []string, work func(context.Context, PackRepairPlan, []string) (int, error)) (int, error) {
	payload := repairTask{Plan: plan, Paths: slices.Clone(paths)}
	intent, err := taskPlanIntent(ctx, s.instances, "repair", "恢复整合包文件", plan.InstanceID, payload)
	if err != nil {
		return 0, err
	}
	return performTask(ctx, s.Tasks, intent, func(task context.Context) (int, error) {
		if err := checkTaskTarget(task, s.instances, intent); err != nil {
			return 0, err
		}
		current, selected := plan, paths
		if TaskAttempt(task) > 1 {
			fresh, names, e := s.repairTaskPlan(task, plan, paths)
			if e != nil {
				return 0, e
			}
			current, selected = fresh, names
			if len(selected) == 0 {
				return 0, nil
			}
		}
		return work(task, current, selected)
	})
}
