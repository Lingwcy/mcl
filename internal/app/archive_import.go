package app

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"mcl/internal/core/archive"
	"mcl/internal/core/download"
	"mcl/internal/core/instance"
	"mcl/internal/core/market"
	"mcl/internal/core/meta"
	"mcl/internal/core/modpack"
	"mcl/internal/core/tasks"
)

type ArchiveImportRequest struct {
	Description                                   *string `json:"description,omitempty"`
	Icon                                          []byte  `json:"icon,omitempty"`
	IconStyle                                     string  `json:"icon_style,omitempty"`
	GameDirectoryID                               *string `json:"game_directory_id,omitempty"`
	Source                                        market.Source
	Kind                                          market.Kind
	ProjectID, VersionID, InstanceID, Name, World string
	LocalArchive                                  *market.File `json:"local_archive,omitempty"`
}
type MissingPackFile struct {
	ProjectID, VersionID, Reason string
	Optional                     bool
}
type ArchiveImportPlan struct {
	Request          ArchiveImportRequest
	Archive          market.File
	ArchivePage      string
	NeedsArchive     bool
	Manifest         modpack.Manifest
	Instance         instance.Instance
	Files            []modpack.File
	Missing          []MissingPackFile
	SelectedOptional []string
	Supplied         []string
	WorldFiles       []archive.Mapping
	Warnings         []string
}
type ArchiveStore interface {
	CheckGameDirectory(context.Context, string) error
	InspectPack(context.Context, market.File) (modpack.Manifest, error)
	InspectWorld(context.Context, market.File) ([]archive.Mapping, error)
	CommitArchive(context.Context, ArchiveImportPlan) error
	CheckWorld(context.Context, string, string) error
	SupplyPackFiles(context.Context, []modpack.File, []string) (LocalFileSupply, error)
	CachedManualFiles(context.Context, []modpack.File) ([]string, error)
	InspectPackRepair(context.Context, instance.Instance) (PackRepairPlan, error)
	CommitPackRepair(context.Context, instance.Instance, string, market.File, []PackRepairEntry) error
}

type ArchiveImporter struct {
	Tasks     *TaskService
	catalog   *MarketService
	metadata  VersionCatalog
	instances InstanceReader
	registry  SessionRegistry
	store     ArchiveStore
	downloads *download.Client
}

func NewArchiveImporter(catalog *MarketService, metadata VersionCatalog, instances InstanceReader, registry SessionRegistry, store ArchiveStore, downloads *download.Client) *ArchiveImporter {
	return &ArchiveImporter{catalog: catalog, metadata: metadata, instances: instances, registry: registry, store: store, downloads: downloads}
}
func (s *ArchiveImporter) Inspect(ctx context.Context, request ArchiveImportRequest, settings Settings, report func(download.Progress)) (ArchiveImportPlan, error) {
	if s.Tasks != nil && taskScope(ctx) == nil {
		file, _, _, err := s.resolveArchive(ctx, request)
		if err != nil {
			return ArchiveImportPlan{}, err
		}
		directory := settings.GameDirectoryID
		if request.GameDirectoryID != nil {
			directory = *request.GameDirectoryID
		}
		intent := tasks.Intent{Kind: "archive", Name: "读取整合包 / 地图", Payload: taskPayload(archiveTask{Request: request, Archive: file, Directory: directory})}
		return performTask(ctx, s.Tasks, intent, func(task context.Context) (ArchiveImportPlan, error) {
			return s.Inspect(task, request, settings, report)
		})
	}
	if request.Kind == market.Modpack {
		selected := settings.GameDirectoryID
		if request.GameDirectoryID != nil {
			selected = *request.GameDirectoryID
		}
		if selected != "" && !instance.ValidID(selected) {
			return ArchiveImportPlan{}, errors.New("整合包游戏目录标识无效")
		}
		request.GameDirectoryID = &selected
		settings.GameDirectoryID = selected
	}
	plan := ArchiveImportPlan{Request: request}
	if request.Kind == market.Modpack {
		if err := s.store.CheckGameDirectory(ctx, settings.GameDirectoryID); err != nil {
			return plan, fmt.Errorf("检查游戏安装位置: %w", err)
		}
	}
	if request.Kind != market.Modpack && request.Kind != market.World {
		return plan, errors.New("不是可导入的压缩包类型")
	}
	client := s.catalog.Client()
	file, page, version, err := s.resolveArchive(ctx, request)
	if err != nil {
		return plan, err
	}
	if err := s.checkArchiveTask(ctx, file); err != nil {
		return plan, err
	}
	if err := TaskStage(ctx, "读取压缩包与清单"); err != nil {
		return plan, err
	}
	plan.ArchivePage = page
	plan.Archive = file
	ext := strings.ToLower(path.Ext(file.Filename))
	if ext != ".zip" && !(request.Kind == market.Modpack && ext == ".mrpack") {
		return plan, errors.New("不支持此压缩包扩展名")
	}
	if request.Kind == market.World {
		value, err := s.instances.Get(ctx, request.InstanceID)
		if err != nil {
			return plan, err
		}
		if _, err := market.VersionFileMetadata(version, request.ProjectID, value.Version.ID); err != nil {
			return plan, err
		}
		plan.Instance = value
		if err := s.store.CheckWorld(ctx, request.InstanceID, request.World); err != nil {
			return plan, err
		}
	} else {
		if err := settings.Validate(); err != nil {
			return plan, err
		}
		if err := instance.ValidateName(strings.TrimSpace(request.Name)); err != nil {
			return plan, err
		}
	}
	if file.URL == "" {
		ready, err := s.store.CachedManualFiles(ctx, []modpack.File{archiveSupplyFile(file)})
		if err != nil {
			return plan, err
		}
		if len(ready) == 0 {
			plan.NeedsArchive = true
			return plan, nil
		}
	} else if err := s.downloads.Run(ctx, []download.File{archiveDownload(file)}, report); err != nil {
		return plan, err
	}
	if request.Kind == market.World {
		plan.WorldFiles, err = s.store.InspectWorld(ctx, file)
		return plan, err
	}
	manifest, err := s.store.InspectPack(ctx, file)
	if err != nil {
		return plan, err
	}
	if request.LocalArchive != nil && manifest.Format != string(request.Source) {
		return plan, errors.New("本地整合包格式与固定记录不匹配")
	}
	plan.Manifest = manifest
	versions, err := s.metadata.Manifest(ctx, false)
	if err != nil {
		return plan, err
	}
	entry, found := versions.Manifest.Find(manifest.Minecraft)
	if !found || (entry.Type != "release" && entry.Type != "snapshot") {
		return plan, errors.New("整合包要求的官方 Minecraft 版本尚不可用")
	}
	game, err := s.metadata.Version(ctx, entry)
	if err != nil {
		return plan, err
	}
	if err := game.ValidateVanilla(); err != nil {
		return plan, err
	}
	if err := manifest.CheckRuntime(game); err != nil {
		return plan, err
	}
	value := instance.Instance{ID: instance.NewID(), Name: strings.TrimSpace(request.Name), Version: entry, RequiredJava: game.Java.Major, JavaPath: settings.JavaPath, GameDirectoryID: settings.GameDirectoryID, CreatedAt: time.Now().UTC(), Pack: &instance.Pack{Source: string(request.Source), ProjectID: request.ProjectID, VersionID: request.VersionID, ArchiveSHA1: file.SHA1, Local: request.LocalArchive != nil}}
	value.Description = manifest.Summary
	if request.Description != nil {
		value.Description = strings.TrimSpace(*request.Description)
	}
	value.IconStyle = request.IconStyle
	value.Icon, err = normalizeInstanceIcon(request.Icon)
	if err != nil {
		return plan, err
	}
	value.Pack.Name, value.Pack.Version, value.Pack.Author = manifest.Name, manifest.Version, manifest.Author
	for _, loader := range manifest.Loaders {
		value.Pack.Loaders = append(value.Pack.Loaders, instance.Loader{Type: loader.Type, Version: loader.Version})
	}
	if err := value.Validate(); err != nil {
		return plan, err
	}
	plan.Instance = value
	if required, ok := value.RequiredLoader(); ok && required.Type == "forge" && game.Java.Major == 8 {
		plan.Warnings = append(plan.Warnings, "Forge 安装器将在导入后核验；文件可导入不代表游戏已可运行。")
	}
	var total int64
	for _, f := range manifest.Files {
		if err := ctx.Err(); err != nil {
			return plan, err
		}
		if manifest.Format == "curseforge" {
			resolved, fetchErr := resolveCursePackFile(ctx, client, manifest.Minecraft, f)
			if fetchErr != nil {
				if ctx.Err() != nil {
					return plan, ctx.Err()
				}
				plan.Missing = append(plan.Missing, MissingPackFile{ProjectID: f.ProjectID, VersionID: f.VersionID, Optional: f.Optional, Reason: fetchErr.Error()})
				continue
			}
			f = resolved
		}
		total += f.Size
		if total > 8<<30 {
			return plan, errors.New("整合包下载体积超过 8 GiB 上限")
		}
		plan.Files = append(plan.Files, f)
	}
	if err := validatePackTargets(plan); err != nil {
		return plan, err
	}
	slices.SortStableFunc(plan.Files, func(a, b modpack.File) int {
		if (len(a.URLs) == 0) == (len(b.URLs) == 0) {
			return 0
		}
		if len(a.URLs) == 0 {
			return -1
		}
		return 1
	})
	plan.Supplied, err = s.store.CachedManualFiles(ctx, plan.Files)
	if err != nil {
		return plan, err
	}
	return plan, nil
}

func resolveCursePackFile(ctx context.Context, client *market.Client, game string, entry modpack.File) (modpack.File, error) {
	remote, err := client.Version(ctx, market.CurseForge, entry.ProjectID, entry.VersionID)
	if err != nil {
		return entry, err
	}
	if remote.Source != market.CurseForge || remote.ID != entry.VersionID {
		return entry, errors.New("文件版本身份不匹配")
	}
	file, err := market.VersionFileMetadata(remote, entry.ProjectID, game)
	if err != nil {
		return entry, err
	}
	project, err := client.ProjectInfo(ctx, market.CurseForge, entry.ProjectID, "")
	if err != nil {
		return entry, err
	}
	folder, extension := "mods", ".jar"
	switch project.Kind {
	case market.Mod:
	case market.ResourcePack:
		folder, extension = "resourcepacks", ".zip"
	case market.Shader:
		folder, extension = "shaderpacks", ".zip"
	default:
		return entry, errors.New("CurseForge 整合包文件类别不受支持")
	}
	if !strings.EqualFold(path.Ext(file.Filename), extension) {
		return entry, errors.New("整合包文件类型与类别不一致")
	}
	entry.Path, entry.SHA1, entry.Size = path.Join(folder, file.Filename), strings.ToLower(file.SHA1), file.Size
	if !project.DistributionRestricted && modpack.AllowedDownload(file.URL) {
		entry.URLs = []string{file.URL}
	}
	if market.SafeURL(project.PageURL) {
		entry.PageURL = project.PageURL
	}
	return entry, nil
}

func (s *ArchiveImporter) Supply(ctx context.Context, plan ArchiveImportPlan, paths []string) (LocalFileSupply, error) {
	if plan.NeedsArchive && (plan.Request.Kind == market.Modpack || plan.Request.Kind == market.World) {
		if len(paths) == 0 {
			return LocalFileSupply{}, ctx.Err()
		}
		file := plan.Archive
		if len(paths) != 1 || file.URL != "" || !market.SafeFilename(file.Filename) || !meta.ValidSHA1(file.SHA1) || file.Size <= 0 || file.Size > modpack.MaxArchiveSize {
			return LocalFileSupply{}, errors.New("请选择固定版本对应的压缩包")
		}
		return s.store.SupplyPackFiles(ctx, []modpack.File{archiveSupplyFile(file)}, paths)
	}
	if plan.Request.Kind != market.Modpack {
		return LocalFileSupply{}, errors.New("请选择整合包文件清单")
	}
	if err := validatePackTargets(plan); err != nil {
		return LocalFileSupply{}, err
	}
	return s.store.SupplyPackFiles(ctx, plan.Files, paths)
}

func archiveSupplyFile(file market.File) modpack.File {
	return modpack.File{Path: file.Filename, SHA1: file.SHA1, Size: file.Size}
}

func (plan ArchiveImportPlan) ActiveFiles() []modpack.File {
	overrides := make(map[string]bool, len(plan.Manifest.Overrides))
	for _, override := range plan.Manifest.Overrides {
		overrides[override.Target] = true
	}
	optional := make(map[string]bool, len(plan.SelectedOptional))
	for _, name := range plan.SelectedOptional {
		optional[name] = true
	}
	var files []modpack.File
	for _, file := range plan.Files {
		if !overrides[file.Path] && (!file.Optional || optional[file.Path]) {
			files = append(files, file)
		}
	}
	return files
}

func (plan ArchiveImportPlan) MissingRequired() int {
	count := 0
	for _, missing := range plan.Missing {
		if !missing.Optional {
			count++
		}
	}
	supplied := make(map[string]bool, len(plan.Supplied))
	for _, name := range plan.Supplied {
		supplied[name] = true
	}
	for _, file := range plan.ActiveFiles() {
		if file.Size > 0 && len(file.URLs) == 0 && !supplied[file.Path] {
			count++
		}
	}
	return count
}
func archiveDownload(file market.File) download.File {
	return download.File{Path: path.Join("cache", "market", file.SHA1), Download: meta.Download{URL: file.URL, SHA1: file.SHA1, Size: file.Size}, AllowedHosts: modpack.DownloadHosts()}
}
func packFileDownload(f modpack.File) download.File {
	return download.File{Path: path.Join("cache", "market", f.SHA1), Download: meta.Download{URL: f.URLs[0], SHA1: f.SHA1, Size: f.Size}, SHA512: f.SHA512, AllowedHosts: modpack.DownloadHosts()}
}
func validatePackTargets(plan ArchiveImportPlan) error {
	if len(plan.Files) > modpack.MaxFiles || len(plan.Supplied) > modpack.MaxFiles {
		return errors.New("整合包文件数量超出限制")
	}
	seen := make(map[string]string)
	nodes := make(map[string]bool)
	var total int64
	for _, f := range plan.Files {
		if !archive.SafePath(f.Path) || !meta.ValidSHA1(f.SHA1) || f.Size < 0 || f.Size > download.MaxFileSize || len(f.URLs) > 8 {
			return errors.New("整合包文件身份或目标路径无效")
		}
		if f.SHA512 != "" {
			decoded, err := hex.DecodeString(f.SHA512)
			if err != nil || len(decoded) != 64 {
				return errors.New("整合包文件 SHA-512 无效")
			}
		}
		for _, address := range f.URLs {
			if !modpack.AllowedDownload(address) {
				return errors.New("整合包文件下载来源无效")
			}
		}
		total += f.Size
		if total > 8<<30 {
			return errors.New("整合包文件超过 8 GiB")
		}
		key := archive.Key(f.Path)
		if _, ok := seen[key]; ok {
			return errors.New("整合包文件发生目标冲突")
		}
		seen[key] = f.Path
	}
	for _, f := range plan.Manifest.Overrides {
		key := archive.Key(f.Target)
		if old, ok := seen[key]; ok && old != f.Target {
			return errors.New("整合包覆盖路径大小写冲突")
		}
		seen[key] = f.Target
	}
	for _, name := range seen {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			nodes[archive.Key(parent)] = true
		}
	}
	for key := range seen {
		if nodes[key] {
			return errors.New("整合包文件与目录冲突")
		}
	}
	return nil
}
func (s *ArchiveImporter) Import(ctx context.Context, plan ArchiveImportPlan, report func(download.Progress)) (instance.Instance, error) {
	if plan.NeedsArchive {
		return instance.Instance{}, errors.New("请先补齐原始压缩包")
	}
	if plan.Request.Kind == market.Modpack {
		plan.Instance.Name = strings.TrimSpace(plan.Request.Name)
		if plan.Request.Description != nil {
			plan.Instance.Description = strings.TrimSpace(*plan.Request.Description)
		}
		plan.Instance.IconStyle = plan.Request.IconStyle
		var err error
		plan.Instance.Icon, err = normalizeInstanceIcon(plan.Request.Icon)
		if err != nil {
			return instance.Instance{}, err
		}
	}
	if err := plan.Instance.Validate(); err != nil {
		return instance.Instance{}, err
	}
	if plan.Request.Kind == market.World {
		lease, err := s.registry.Reserve(ctx, plan.Request.InstanceID)
		if err != nil {
			return instance.Instance{}, err
		}
		defer lease.Close()
		value, err := s.instances.Get(ctx, plan.Request.InstanceID)
		if err != nil {
			return value, err
		}
		if value.Version.SHA1 != plan.Instance.Version.SHA1 {
			return value, errors.New("目标实例版本已改变")
		}
		if err := checkTaskInstance(ctx, value); err != nil {
			return value, err
		}
		if taskScope(ctx) != nil {
			if checker, ok := s.store.(interface {
				TaskWorldReady(context.Context, ArchiveImportPlan) (bool, error)
			}); ok {
				ready, e := checker.TaskWorldReady(ctx, plan)
				if e != nil {
					return value, e
				}
				if ready {
					return value, nil
				}
			}
		}
		if err := s.store.CheckWorld(ctx, plan.Request.InstanceID, plan.Request.World); err != nil {
			return value, err
		}
		if plan.Archive.URL != "" {
			if err := s.downloads.Run(ctx, []download.File{archiveDownload(plan.Archive)}, report); err != nil {
				return value, err
			}
		}
		if err := TaskStage(ctx, "发布"); err != nil {
			return value, err
		}
		return value, s.store.CommitArchive(ctx, plan)
	}
	if plan.Request.Kind != market.Modpack || plan.Instance.Pack == nil {
		return instance.Instance{}, errors.New("无效整合包安装计划")
	}
	if err := validatePackTargets(plan); err != nil {
		return instance.Instance{}, err
	}
	if count := plan.MissingRequired(); count > 0 {
		return instance.Instance{}, fmt.Errorf("整合包仍有 %d 个必需文件待补齐", count)
	}
	supplied := make(map[string]bool, len(plan.Supplied))
	for _, name := range plan.Supplied {
		supplied[name] = true
	}
	var files []download.File
	if plan.Archive.URL != "" {
		files = append(files, archiveDownload(plan.Archive))
	}
	for _, file := range plan.ActiveFiles() {
		if file.Size == 0 || supplied[file.Path] {
			continue
		}
		files = append(files, packFileDownload(file))
	}
	if err := s.downloads.Run(ctx, files, report); err != nil {
		return instance.Instance{}, err
	}
	if err := TaskStage(ctx, "发布"); err != nil {
		return instance.Instance{}, err
	}
	if err := s.store.CommitArchive(ctx, plan); err != nil {
		return instance.Instance{}, err
	}
	return plan.Instance, nil
}
