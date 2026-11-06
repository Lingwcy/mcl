package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"mcl/internal/core/download"
	"mcl/internal/core/market"
	"mcl/internal/core/mods"
)

type AssetDiagnosticSnapshot struct {
	Input     market.DiagnosticInput
	Truncated bool
	Warnings  []string
	Local     []mods.File
}

type AssetDiagnosticResult struct {
	InstanceID string
	Report     market.DiagnosticReport
	ScannedAt  time.Time
	Warnings   []string
}

type AssetDiagnosticRepository interface {
	DiagnosticSnapshot(context.Context, string, func(download.Progress)) (AssetDiagnosticSnapshot, error)
}

type AssetDiagnosticService struct {
	instances InstanceReader
	registry  SessionRegistry
	store     AssetDiagnosticRepository
	probe     RuntimeProbe
}

func NewAssetDiagnosticService(instances InstanceReader, registry SessionRegistry, store AssetDiagnosticRepository, probe RuntimeProbe) *AssetDiagnosticService {
	return &AssetDiagnosticService{instances: instances, registry: registry, store: store, probe: probe}
}

// Diagnose is an explicit offline task. The instance lease excludes managed
// writes and running games while hashing; no catalog requests or writes occur.
func (s *AssetDiagnosticService) Diagnose(ctx context.Context, id string, report func(download.Progress)) (AssetDiagnosticResult, error) {
	result := AssetDiagnosticResult{InstanceID: id}
	if s.registry == nil {
		return result, errors.New("实例会话保护未配置")
	}
	lease, err := s.registry.Reserve(ctx, id)
	if err != nil {
		return result, err
	}
	defer lease.Close()
	value, err := s.instances.Get(ctx, id)
	if err != nil {
		return result, err
	}
	snapshot, err := s.store.DiagnosticSnapshot(ctx, id, report)
	if err != nil {
		return result, err
	}
	snapshot.Input.GameVersion = value.Version.ID
	if loader, ok := value.RequiredLoader(); ok {
		snapshot.Input.Loader = loader.Type
	}
	result.Report, err = market.Diagnose(ctx, snapshot.Input)
	if err != nil {
		return result, err
	}
	runtime := mods.Runtime{GameVersion: value.Version.ID}
	if loader, ok := value.RequiredLoader(); ok {
		runtime.Loader, runtime.LoaderVersion = loader.Type, loader.Version
	}
	if value.JavaPath != "" && s.probe != nil {
		java, probeErr := s.probe(ctx, value.JavaPath)
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if probeErr == nil {
			runtime.JavaVersion = java.Version
			if java.Major <= 8 {
				runtime.JavaVersion = strings.ReplaceAll(strings.TrimPrefix(java.Version, "1."), "_", ".")
			}
		} else {
			snapshot.Warnings = append(snapshot.Warnings, "Java 版本未确认："+probeErr.Error())
		}
	}
	local, err := mods.Check(ctx, snapshot.Local, runtime, snapshot.Truncated)
	if err != nil {
		return result, err
	}
	for _, issue := range local.Issues {
		result.Report.Issues = append(result.Report.Issues, market.AssetIssue{ID: fmt.Sprintf("local\x00%s\x00%s\x00%s\x00%s", issue.Path, issue.ModID, issue.Code, issue.Detail), Code: market.IssueLocalDeclaration, Path: issue.Path, PeerPath: issue.PeerPath, Error: issue.Error, Title: issue.Title, Detail: issue.Detail, LocalID: issue.TargetID, CanEnable: issue.CanEnable})
	}
	slices.SortStableFunc(result.Report.Issues, func(a, b market.AssetIssue) int {
		if a.Error != b.Error {
			if a.Error {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Path+a.ID, b.Path+b.ID)
	})
	if len(result.Report.Issues) > 1000 {
		result.Report.Issues, result.Report.Truncated = result.Report.Issues[:1000], true
	}
	result.Report.Errors = 0
	for _, issue := range result.Report.Issues {
		if issue.Error {
			result.Report.Errors++
		}
	}
	result.Report.Truncated = result.Report.Truncated || local.Truncated
	result.Report.Truncated = result.Report.Truncated || snapshot.Truncated
	result.Warnings = snapshot.Warnings
	if err == nil {
		result.ScannedAt = time.Now().UTC()
	}
	return result, err
}
