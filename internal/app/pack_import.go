package app

import (
	"context"
	"errors"
	"fmt"

	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
)

// PackImportResult keeps file publication distinct from runtime readiness. A
// later install failure must not hide the created instance or import it twice.
type PackImportResult struct {
	Instance               instance.Instance
	Receipt                install.Receipt
	Imported, RuntimeReady bool
}
type PackImportService struct {
	archives      *ArchiveImporter
	installations *InstallationService
	java          JavaResolver
}

func NewPackImportService(archives *ArchiveImporter, installations *InstallationService, java JavaResolver) *PackImportService {
	return &PackImportService{archives: archives, installations: installations, java: java}
}
func (s *PackImportService) Run(ctx context.Context, plan ArchiveImportPlan, report func(install.Progress)) (PackImportResult, error) {
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
