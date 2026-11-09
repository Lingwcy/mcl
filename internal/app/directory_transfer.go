package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"mcl/internal/core/discovery"
	"mcl/internal/core/instance"
	"mcl/internal/core/transfer"
)

type DirectoryImportRequest struct {
	SourceID, CandidateID, DataPath, Name, DirectoryID string
	Groups                                             []string
}
type DirectoryPlan struct {
	Kind, Token, SourceID, CandidateID, DataPath, SourceFingerprint string
	Before, After                                                   instance.Instance
	Tree                                                            transfer.Tree
}

func (p DirectoryPlan) Fingerprint() string {
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type DirectoryOperation struct {
	InstanceID, Name, Kind, Token, Phase string
	From, To, SourceID                   string
	Bytes                                int64
	Backup, Changed                      bool
}
type DirectoryRepository interface {
	InspectSource(context.Context, DirectoryImportRequest, func(transfer.Progress)) (discovery.Candidate, string, transfer.Tree, error)
	PlanMove(context.Context, string, string, func(transfer.Progress)) (DirectoryPlan, error)
	CheckLocation(context.Context, string) error
	GamePath(context.Context, instance.Instance) (string, error)
	Begin(context.Context, DirectoryPlan) error
	Pending(context.Context, string) (DirectoryPlan, error)
	ListOperations(context.Context) ([]DirectoryOperation, error)
	Lock(context.Context, string) (io.Closer, error)
	Run(context.Context, DirectoryPlan, func(transfer.Progress)) (instance.Instance, error)
	Abort(context.Context, string, string, func(string) error) error
	CleanBackup(context.Context, string, string, func(string) error) error
	BackupDirectory(context.Context, string, string) (string, error)
}
type DirectoryRegistry interface {
	ReserveForDirectoryTransfer(context.Context, string) (SessionLease, error)
}
type DirectoryService struct {
	store    DirectoryRepository
	registry DirectoryRegistry
	catalog  VersionCatalog
	loaders  LoaderReleaseCatalog
	trash    func(string) error
	slot     chan struct{}
}

func NewDirectoryService(store DirectoryRepository, registry DirectoryRegistry, catalog VersionCatalog, loaders LoaderReleaseCatalog, trash func(string) error) *DirectoryService {
	return &DirectoryService{store: store, registry: registry, catalog: catalog, loaders: loaders, trash: trash, slot: make(chan struct{}, 1)}
}
func (s *DirectoryService) InspectImport(ctx context.Context, request DirectoryImportRequest, report func(transfer.Progress)) (DirectoryPlan, error) {
	var plan DirectoryPlan
	request.Name = strings.TrimSpace(request.Name)
	request.Groups = slices.Clone(request.Groups)
	if err := instance.ValidateName(request.Name); err != nil {
		return plan, err
	}
	if err := s.store.CheckLocation(ctx, request.DirectoryID); err != nil {
		return plan, err
	}
	candidate, fingerprint, tree, err := s.store.InspectSource(ctx, request, report)
	if err != nil {
		return plan, err
	}
	if candidate.Status == "unreadable" || candidate.GameVersion == "" || candidate.UnknownComponents || (len(candidate.Loaders) == 0 && candidate.MainClass != "net.minecraft.client.main.Main" && candidate.MainClass != "net.minecraft.client.Minecraft") {
		return plan, errors.New("来源基底或运行组件无法明确识别，暂不支持这类来源的受管复制")
	}
	catalog, err := s.catalog.Manifest(ctx, false)
	if err != nil {
		return plan, err
	}
	entry, ok := catalog.Manifest.Find(candidate.GameVersion)
	if !ok {
		return plan, errors.New("找不到来源声明对应的官方游戏版本")
	}
	version, err := s.catalog.Version(ctx, entry)
	if err != nil {
		return plan, err
	}
	if err := version.ValidateVanilla(); err != nil {
		return plan, err
	}
	value := instance.Instance{ID: instance.NewID(), Name: request.Name, Version: entry, RequiredJava: version.Java.Major, CreatedAt: time.Now().UTC(), GameDirectoryID: request.DirectoryID}
	if len(candidate.Loaders) > 1 {
		return plan, errors.New("来源含多项加载器声明，不能猜测运行环境")
	}
	if len(candidate.Loaders) == 1 {
		hint := candidate.Loaders[0]
		build := hint.Version
		if hint.Kind == "forge" || hint.Kind == "neoforge" && entry.ID == "1.20.1" {
			build = strings.TrimPrefix(build, entry.ID+"-")
		}
		required := instance.Loader{Type: hint.Kind, Version: build}
		if !instance.ValidLoader(required) {
			return plan, errors.New("来源加载器身份不受支持")
		}
		if s.loaders == nil {
			return plan, errors.New("加载器官方目录服务未配置")
		}
		rows, err := s.loaders.Releases(ctx, required.Type, entry.ID)
		if err != nil {
			return plan, err
		}
		found := false
		for _, row := range rows {
			if row.Version == required.Version {
				found = true
			}
		}
		if !found {
			return plan, errors.New("来源加载器版本不在对应游戏的官方目录中")
		}
		value.Loader = &required
	}
	plan = DirectoryPlan{Kind: "import", Token: instance.NewID(), SourceID: request.SourceID, CandidateID: request.CandidateID, DataPath: request.DataPath, SourceFingerprint: fingerprint, After: value, Tree: tree}
	return plan, value.Validate()
}
func (s *DirectoryService) InspectMove(ctx context.Context, id, to string, report func(transfer.Progress)) (DirectoryPlan, error) {
	guard, err := s.store.Lock(ctx, id)
	if err != nil {
		return DirectoryPlan{}, err
	}
	defer guard.Close()
	lease, err := s.registry.ReserveForDirectoryTransfer(ctx, id)
	if err != nil {
		return DirectoryPlan{}, err
	}
	defer lease.Close()
	if _, e := s.store.Pending(ctx, id); e == nil {
		return DirectoryPlan{}, errors.New("已有未完成目录操作，请打开目录任务继续或撤销")
	} else if !errors.Is(e, os.ErrNotExist) {
		return DirectoryPlan{}, e
	}
	return s.store.PlanMove(ctx, id, to, report)
}
func (s *DirectoryService) Paths(ctx context.Context, plan DirectoryPlan) (string, string, error) {
	source := plan.DataPath
	if plan.Kind == "move" {
		name, err := s.store.GamePath(ctx, plan.Before)
		if err != nil {
			return "", "", err
		}
		source = name
	}
	target, err := s.store.GamePath(ctx, plan.After)
	return source, target, err
}
func (s *DirectoryService) BackupDirectory(ctx context.Context, id, token string) (string, error) {
	return s.store.BackupDirectory(ctx, id, token)
}
func (s *DirectoryService) List(ctx context.Context) ([]DirectoryOperation, error) {
	return s.store.ListOperations(ctx)
}
func (s *DirectoryService) Run(ctx context.Context, plan DirectoryPlan, report func(transfer.Progress)) (instance.Instance, error) {
	select {
	case s.slot <- struct{}{}:
		defer func() { <-s.slot }()
	case <-ctx.Done():
		return instance.Instance{}, ctx.Err()
	}
	guard, err := s.store.Lock(ctx, plan.After.ID)
	if err != nil {
		return instance.Instance{}, err
	}
	defer guard.Close()
	if plan.Kind == "import" {
		catalog, err := s.catalog.Manifest(ctx, false)
		if err != nil {
			return instance.Instance{}, err
		}
		entry, ok := catalog.Manifest.Find(plan.After.Version.ID)
		if !ok || entry.SHA1 != plan.After.Version.SHA1 || entry.URL != plan.After.Version.URL {
			return instance.Instance{}, errors.New("导入的官方版本身份已改变，请重新预览")
		}
		base, err := s.catalog.Version(ctx, entry)
		if err != nil {
			return instance.Instance{}, err
		}
		if err := base.ValidateVanilla(); err != nil {
			return instance.Instance{}, err
		}
		if base.Java.Major != plan.After.RequiredJava {
			return instance.Instance{}, errors.New("游戏 Java 要求与固定导入计划不符")
		}
		if required, ok := plan.After.RequiredLoader(); ok {
			if s.loaders == nil {
				return instance.Instance{}, errors.New("加载器官方目录服务未配置")
			}
			releases, err := s.loaders.Releases(ctx, required.Type, entry.ID)
			if err != nil {
				return instance.Instance{}, err
			}
			found := false
			for _, row := range releases {
				if row.Version == required.Version {
					found = true
				}
			}
			if !found {
				return instance.Instance{}, errors.New("固定加载器版本不在官方目录中，未提交复制")
			}
		}

	}
	var lease SessionLease
	if plan.Kind == "move" {
		lease, err = s.registry.ReserveForDirectoryTransfer(ctx, plan.After.ID)
		if err != nil {
			return instance.Instance{}, err
		}
		defer lease.Close()
	}
	if err := s.store.Begin(ctx, plan); err != nil {
		return instance.Instance{}, err
	}
	if plan.Kind == "import" {
		lease, err = s.registry.ReserveForDirectoryTransfer(ctx, plan.After.ID)
		if err != nil {
			return instance.Instance{}, err
		}
		defer lease.Close()
	}
	value, err := s.store.Run(ctx, plan, report)
	if err != nil {
		return value, fmt.Errorf("目录操作未完成，源数据已保留；可在目录任务中继续或撤销: %w", err)
	}
	return value, nil
}
func (s *DirectoryService) Resume(ctx context.Context, id, token string, report func(transfer.Progress)) (instance.Instance, error) {
	plan, err := s.store.Pending(ctx, id)
	if err != nil {
		return instance.Instance{}, err
	}
	if plan.Token != token {
		return instance.Instance{}, errors.New("目录任务已改变，请重新读取")
	}
	return s.Run(ctx, plan, report)
}
func (s *DirectoryService) Abort(ctx context.Context, id, token string) error {
	guard, err := s.store.Lock(ctx, id)
	if err != nil {
		return err
	}
	defer guard.Close()
	lease, err := s.registry.ReserveForDirectoryTransfer(ctx, id)
	if err != nil {
		return err
	}
	// The persistent marker and independent directory guard protect the final
	// Trash call after closing the Windows lock handle inside the metadata root.
	if err := lease.Close(); err != nil {
		return err
	}
	return s.store.Abort(ctx, id, token, s.trash)
}
func (s *DirectoryService) CleanBackup(ctx context.Context, id, token string) error {
	guard, err := s.store.Lock(ctx, id)
	if err != nil {
		return err
	}
	defer guard.Close()
	lease, err := s.registry.ReserveForDirectoryTransfer(ctx, id)
	if err != nil {
		return err
	}
	defer lease.Close()
	return s.store.CleanBackup(ctx, id, token, s.trash)
}
