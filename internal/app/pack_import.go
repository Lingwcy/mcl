package app

import (
	"context"
	"errors"
	"fmt"
	"os"

	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
	"mcl/internal/core/tasks"
)

// PackImportResult keeps file publication distinct from runtime readiness. A
// later install failure must not hide the created instance or import it twice.
type PackImportResult struct {
	Instance               instance.Instance
	Receipt                install.Receipt
	Imported, RuntimeReady bool
}
type PackImportService struct {
	Tasks         *TaskService
	archives      *ArchiveImporter
	installations *InstallationService
	java          JavaResolver
}

func NewPackImportService(archives *ArchiveImporter, installations *InstallationService, java JavaResolver) *PackImportService {
	return &PackImportService{archives: archives, installations: installations, java: java}
}
func (s *PackImportService) Run(ctx context.Context, plan ArchiveImportPlan, report func(install.Progress)) (PackImportResult, error) {
	if s.Tasks != nil && taskScope(ctx) == nil {
		intent := tasks.Intent{Kind: "pack", Name: "导入整合包 · " + plan.Instance.Name, InstanceID: plan.Instance.ID, Payload: taskPayload(plan)}
		if plan.Request.Kind == market.World {
			value, err := s.archives.instances.Get(ctx, plan.Instance.ID)
			if err != nil {
				return PackImportResult{}, err
			}
			intent.Fingerprint = taskInstanceFingerprint(value)
			intent.Name = "导入地图"
		}
		return performTask(ctx, s.Tasks, intent, func(task context.Context) (PackImportResult, error) {
			if err := checkTaskTarget(task, s.archives.instances, intent); err != nil {
				return PackImportResult{}, err
			}
			current := plan
			if TaskAttempt(task) > 1 {
				if _, err := s.archives.instances.Get(task, plan.Instance.ID); errors.Is(err, os.ErrNotExist) || plan.Request.Kind == market.World {
					fresh, e := s.refreshTaskPlan(task, plan)
					if e != nil {
						return PackImportResult{}, e
					}
					current = fresh
				}
			}
			return s.Run(task, current, report)
		})
	}
	if plan.Request.Kind == market.Modpack {
		if current, err := s.archives.instances.Get(ctx, plan.Instance.ID); err == nil {
			if taskInstanceFingerprint(current) != taskInstanceFingerprint(plan.Instance) {
				return PackImportResult{}, errors.New("已导入实例与固定任务身份不符，未重新导入")
			}
			store, ok := s.archives.store.(interface {
				CheckImportedTask(context.Context, instance.Instance, ArchiveImportPlan) error
			})
			if !ok {
				return PackImportResult{}, errors.New("整合包发布身份核验服务未配置")
			}
			if err := store.CheckImportedTask(ctx, current, plan); err != nil {
				return PackImportResult{}, err
			}
			receipt, err := s.installations.RunAutomatic(ctx, current.ID, s.java, report)
			return PackImportResult{Instance: current, Imported: true, RuntimeReady: err == nil, Receipt: receipt}, err
		} else if !errors.Is(err, os.ErrNotExist) {
			return PackImportResult{}, err
		}
	}
	value, err := s.archives.Import(ctx, plan, func(progress download.Progress) {
		if report != nil {
			report(install.Progress{Stage: "modpack", Progress: progress})
		}
	})
	result := PackImportResult{Instance: value, Imported: err == nil}
	if err != nil {
		return result, err
	}
	if plan.Request.Kind == market.World {
		return result, nil
	}
	if err := TaskStage(ctx, "准备整合包运行环境"); err != nil {
		return result, err
	}
	receipt, err := s.installations.RunAutomatic(ctx, value.ID, s.java, report)
	result.Receipt, result.RuntimeReady = receipt, err == nil
	if err != nil {
		return result, fmt.Errorf("实例已导入，运行环境未完成，可继续安装: %w", err)
	}
	return result, nil
}
func (s *PackImportService) Resume(ctx context.Context, id string, report func(install.Progress)) (PackImportResult, error) {
	value, err := s.installations.instances.Get(ctx, id)
	if err != nil {
		return PackImportResult{}, err
	}
	if value.Pack == nil {
		return PackImportResult{}, errors.New("不是已导入的整合包实例")
	}
	receipt, err := s.installations.RunAutomatic(ctx, id, s.java, report)
	return PackImportResult{Instance: value, Imported: true, Receipt: receipt, RuntimeReady: err == nil}, err
}
