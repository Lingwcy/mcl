package market

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/instance"
	"mcl/internal/core/meta"
)

// InstalledFile identifies one physical file and one catalog identity for it.
// File.Path is canonical; Path includes .disabled when it is not active.
type InstalledFile struct {
	File    InstallFile
	Path    string
	Enabled bool
}
type UpdateSelection struct {
	Path                 string
	Source               Source
	ProjectID, VersionID string
}
type UpdateRequest struct {
	InstanceID, GameVersion, VersionSHA1, Loader, World string
	Kind                                                Kind
	Selections                                          []UpdateSelection
}
type UpdateChange struct {
	Before  *InstalledFile
	After   InstallFile
	Enabled bool
}
type UpdatePlan struct {
	Request       UpdateRequest
	ReceiptSHA256 string
	Changes       []UpdateChange
	Reused        []InstalledFile
	Warnings      []string
	LocalPages    map[string]string `json:"-"`
	ReadyLocal    []string          `json:"-"`
}

func (c UpdateChange) ReusesContent() bool {
	return c.Before != nil && c.Before.File.File.Size == c.After.File.Size && strings.EqualFold(c.Before.File.File.SHA1, c.After.File.SHA1)
}

func (p UpdatePlan) LocalFiles() []InstallFile {
	var files []InstallFile
	for _, change := range p.Changes {
		if !change.ReusesContent() && change.After.File.URL == "" {
			files = append(files, change.After)
		}
	}
	return files
}

func (p UpdatePlan) MissingLocal() int {
	count := 0
	for _, change := range p.Changes {
		if !change.ReusesContent() && change.After.File.URL == "" && !slices.Contains(p.ReadyLocal, change.After.Path) {
			count++
		}
	}
	return count
}

func (c UpdateChange) Path() string {
	if c.Enabled {
		return c.After.Path
	}
	return c.After.Path + ".disabled"
}

func AssetDirectory(kind Kind, world string) (string, string, error) {
	switch kind {
	case Mod:
		return "mods", ".jar", nil
	case Shader:
		return "shaderpacks", ".zip", nil
	case ResourcePack:
		return "resourcepacks", ".zip", nil
	case DataPack:
		if SafeFilename(world) && meta.ValidID(world) {
			return path.Join("saves", world, "datapacks"), ".zip", nil
		}
	}
	return "", "", errors.New("不支持此资产更新类型或存档")
}

// BuildUpdate resolves all roots together. Explicit target versions are pinned;
// compatible existing dependencies are retained before selecting new releases.
// The caller verifies the concrete local files and seals its receipt digest.
func BuildUpdate(ctx context.Context, catalog Catalog, request UpdateRequest, installed []InstalledFile) (UpdatePlan, error) {
	plan := UpdatePlan{Request: request}
	directory, extension, err := AssetDirectory(request.Kind, request.World)
	if err != nil {
		return plan, err
	}
	if !instance.ValidID(request.InstanceID) || !meta.ValidID(request.GameVersion) || !meta.ValidSHA1(request.VersionSHA1) || len(request.Selections) == 0 || len(request.Selections) > 64 {
		return plan, errors.New("更新请求身份或数量无效")
	}
	if request.Kind == Mod && request.Loader == "" {
		return plan, errors.New("请先选择此实例的模组加载器")
	}
	projectKey := func(source Source, id string) string { return string(source) + ":" + id }
	byProject := make(map[string][]InstalledFile)
	for _, file := range installed {
		if path.Dir(file.File.Path) != directory {
			continue
		}
		key := projectKey(file.File.Source, file.File.ProjectID)
		byProject[key] = append(byProject[key], file)
	}
	explicit := make(map[string]UpdateSelection, len(request.Selections))
	selectedPaths := make(map[string]bool)
	for _, selection := range request.Selections {
		if (selection.Source != Modrinth && selection.Source != CurseForge) || !ValidID(selection.ProjectID) || !ValidID(selection.VersionID) {
			return plan, errors.New("更新版本身份无效")
		}
		key := projectKey(selection.Source, selection.ProjectID)
		if _, ok := explicit[key]; ok {
			return plan, errors.New("同一项目不能同时选择两个更新版本")
		}
		if selectedPaths[archive.Key(selection.Path)] {
			return plan, errors.New("同一文件不能通过两个来源同时更新")
		}
		selectedPaths[archive.Key(selection.Path)] = true
		found := false
		for _, file := range byProject[key] {
			found = found || file.Path == selection.Path
		}
		if !found {
			return plan, errors.New("所选文件没有匹配的已安装来源，请先识别")
		}
		explicit[key] = selection
	}
	type node struct {
		version           Version
		file              InstallFile
		before            *InstalledFile
		enabled, visiting bool
		children          []Dependency
	}
	nodes := make(map[string]*node)
	var order []string
	compatible := func(version Version) bool {
		return slices.Contains(version.GameVersions, request.GameVersion) && (request.Kind != Mod || slices.Contains(version.Loaders, request.Loader))
	}
	var visit func(Source, string, string, bool, int) (*node, error)
	visit = func(source Source, id, pin string, active bool, depth int) (*node, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if depth > 24 {
			return nil, errors.New("更新依赖深度超出限制")
		}
		key := projectKey(source, id)
		selection, isRoot := explicit[key]
		if isRoot {
			if pin != "" && pin != selection.VersionID {
				return nil, fmt.Errorf("依赖要求 %s，但已选更新版本为 %s", pin, selection.VersionID)
			}
			pin = selection.VersionID
		}
		if existing := nodes[key]; existing != nil {
			if existing.visiting {
				return nil, errors.New("更新计划存在循环依赖")
			}
			if pin != "" && pin != existing.version.ID {
				return nil, errors.New("更新依赖要求不同版本")
			}
			if active && !existing.enabled {
				existing.enabled = true
				for _, dependency := range existing.children {
					if _, err := visit(source, dependency.ProjectID, dependency.VersionID, true, depth+1); err != nil {
						return nil, err
					}
				}
			}
			return existing, nil
		}
		if len(nodes) >= 64 {
			return nil, errors.New("更新计划最多包含 64 个项目")
		}
		n := &node{enabled: active, visiting: true}
		nodes[key] = n
		candidates := byProject[key]
		if len(candidates) > 1 {
			return nil, fmt.Errorf("项目 %s 有多份已有文件，请先处理重复版本", id)
		}
		if len(candidates) == 1 {
			before := candidates[0]
			n.before = &before
			if isRoot && before.Path != selection.Path {
				return nil, errors.New("所选更新文件已经改变")
			}
			// Dependencies already enabled remain enabled; a stopped root does
			// not turn off a dependency used by another active file.
			if !isRoot {
				n.enabled = n.enabled || before.Enabled
			}
		}
		cached := false
		if !isRoot && n.before != nil && (pin == "" || pin == n.before.File.VersionID) && n.before.File.Metadata != nil {
			metadata := n.before.File.Metadata
			version := Version{Source: source, ID: n.before.File.VersionID, ProjectID: id, GameVersions: metadata.GameVersions, Loaders: metadata.Loaders, Dependencies: metadata.Dependencies, Files: []File{n.before.File.File}}
			if compatible(version) {
				n.version = version
				n.file = n.before.File
				cached = true
			}
		}
		if !cached {
			var version Version
			var err error
			if pin != "" {
				version, err = catalog.Version(ctx, source, id, pin)
			} else {
				versions, fetchErr := catalog.Versions(ctx, source, id, request.GameVersion, request.Loader)
				err = fetchErr
				for _, candidate := range versions {
					if candidate.Type == "release" && compatible(candidate) {
						version = candidate
						break
					}
				}
			}
			if err != nil {
				return nil, err
			}
			if version.Source != source || version.ProjectID != id || !ValidID(version.ID) || (pin != "" && version.ID != pin) || !compatible(version) {
				return nil, fmt.Errorf("项目 %s 没有满足游戏版本和加载器要求的目标版本", id)
			}
			file, err := version.PrimaryFileMetadata()
			if err != nil {
				return nil, err
			}
			if strings.ToLower(path.Ext(file.Filename)) != extension {
				return nil, errors.New("更新文件类型不匹配")
			}
			file.SHA1 = strings.ToLower(file.SHA1)
			if !SafeURL(file.URL) {
				file.URL = ""
			}
			metadata := MetadataFromVersion(version)
			if err := metadata.Validate(); err != nil {
				return nil, err
			}
			n.version = version
			n.file = InstallFile{Source: source, ProjectID: id, VersionID: version.ID, Path: path.Join(directory, file.Filename), File: file, Metadata: metadata}
			if version.Environment == "dedicated_server_only" {
				plan.Warnings = append(plan.Warnings, version.Name+" 仅支持独立服务端")
			}
		}
		for _, dependency := range n.version.Dependencies {
			if !dependency.Required {
				continue
			}
			if dependency.ProjectID == "" && dependency.VersionID != "" {
				version, err := catalog.Version(ctx, source, id, dependency.VersionID)
				if err != nil {
					return nil, err
				}
				dependency.ProjectID = version.ProjectID
			}
			if !ValidID(dependency.ProjectID) {
				return nil, errors.New("必需依赖缺少可解析的项目身份")
			}
			n.children = append(n.children, dependency)
			if _, err := visit(source, dependency.ProjectID, dependency.VersionID, n.enabled, depth+1); err != nil {
				return nil, err
			}
		}
		n.visiting = false
		order = append(order, key)
		return n, nil
	}
	for _, selection := range request.Selections {
		var active bool
		for _, file := range byProject[projectKey(selection.Source, selection.ProjectID)] {
			if file.Path == selection.Path {
				active = file.Enabled
			}
		}
		if _, err := visit(selection.Source, selection.ProjectID, selection.VersionID, active, 0); err != nil {
			return UpdatePlan{}, err
		}
	}
	paths := make(map[string]bool)
	var total int64
	for _, key := range order {
		n := nodes[key]
		if n.before != nil && n.file.VersionID == n.before.File.VersionID && strings.EqualFold(n.file.File.SHA1, n.before.File.File.SHA1) && n.file.File.Size == n.before.File.File.Size {
			n.file.Path = n.before.File.Path
			if n.enabled == n.before.Enabled {
				plan.Reused = append(plan.Reused, *n.before)
				continue
			}
		}
		change := UpdateChange{Before: n.before, After: n.file, Enabled: n.enabled}
		if paths[archive.Key(change.Path())] {
			return UpdatePlan{}, errors.New("更新文件目标发生冲突")
		}
		paths[archive.Key(change.Path())] = true
		total += change.After.File.Size
		if total > 8<<30 {
			return UpdatePlan{}, errors.New("单次更新文件超过 8 GiB")
		}
		plan.Changes = append(plan.Changes, change)
	}
	return plan, nil
}

func (p UpdatePlan) Validate() error {
	r := p.Request
	directory, extension, err := AssetDirectory(r.Kind, r.World)
	if err != nil {
		return err
	}
	if !instance.ValidID(r.InstanceID) || !meta.ValidID(r.GameVersion) || !meta.ValidSHA1(r.VersionSHA1) || len(r.Selections) == 0 || len(r.Selections) > 64 || len(p.Changes) > 64 || len(p.Reused) > 64 || len(p.Changes)+len(p.Reused) > 64 || len(p.ReceiptSHA256) != 64 {
		return errors.New("固定更新计划身份或数量无效")
	}
	if _, err := hex.DecodeString(p.ReceiptSHA256); err != nil {
		return errors.New("更新来源记录摘要无效")
	}
	validFile := func(file InstallFile) bool {
		return (file.Source == Modrinth || file.Source == CurseForge) && ValidID(file.ProjectID) && ValidID(file.VersionID) && meta.ValidPath(file.Path) && path.Dir(file.Path) == directory && strings.EqualFold(path.Ext(file.Path), extension) && SafeFilename(file.File.Filename) && meta.ValidSHA1(file.File.SHA1) && file.File.Size > 0 && file.File.Size <= 2<<30 && file.Metadata.Validate() == nil
	}
	validBefore := func(file InstalledFile) bool {
		name := file.File.Path
		if !file.Enabled {
			name += ".disabled"
		}
		return validFile(file.File) && file.Path == name
	}
	old, newPaths := make(map[string]bool), make(map[string]bool)
	canonicalTargets := make(map[string]bool)
	var total int64
	for _, change := range p.Changes {
		if !validFile(change.After) {
			return errors.New("更新文件字段或目录无效")
		}
		if change.Before != nil {
			if !validBefore(*change.Before) || old[archive.Key(change.Before.Path)] {
				return errors.New("被替换的旧文件无效或重复")
			}
			old[archive.Key(change.Before.Path)] = true
		}
		if !change.ReusesContent() && ((change.After.File.URL != "" && !SafeURL(change.After.File.URL)) || path.Base(change.After.Path) != change.After.File.Filename) {
			return errors.New("新版本的下载地址或目标文件名无效")
		}
		if newPaths[archive.Key(change.Path())] || canonicalTargets[archive.Key(change.After.Path)] {
			return errors.New("新版本目标文件名重复")
		}
		newPaths[archive.Key(change.Path())] = true
		canonicalTargets[archive.Key(change.After.Path)] = true
		total += change.After.File.Size
	}
	if total > 8<<30 {
		return errors.New("单次更新文件超过 8 GiB")
	}
	for _, file := range p.Reused {
		if !validBefore(file) || old[archive.Key(file.Path)] || newPaths[archive.Key(file.Path)] || canonicalTargets[archive.Key(file.File.Path)] {
			return errors.New("复用文件与更新目标冲突")
		}
	}
	return nil
}

// CheckUpdateRelations rejects newly introduced known dependency failures. It
// preserves evidence of pre-existing problems for a separate diagnosis instead
// of making an unrelated update pretend to have repaired them.
func CheckUpdateRelations(before []InstalledFile, plan UpdatePlan) error {
	removed := make(map[string]bool)
	for _, change := range plan.Changes {
		if change.Before != nil {
			removed[archive.Key(change.Before.Path)] = true
		}
	}
	after := make([]InstalledFile, 0, len(before)+len(plan.Changes))
	for _, file := range before {
		if !removed[archive.Key(file.Path)] {
			after = append(after, file)
		}
	}
	for _, change := range plan.Changes {
		after = append(after, InstalledFile{File: change.After, Path: change.Path(), Enabled: change.Enabled})
	}
	satisfied := func(source Source, dependency Dependency, files []InstalledFile) bool {
		for _, file := range files {
			if file.Enabled && DependencyMatches(source, dependency, file.File) {
				return true
			}
		}
		return false
	}
	changed := make(map[string]bool)
	for _, change := range plan.Changes {
		changed[archive.Key(change.Path())] = true
	}
	for _, file := range after {
		if !file.Enabled || file.File.Metadata == nil {
			continue
		}
		for _, dependency := range file.File.Metadata.Dependencies {
			if !dependency.Required {
				continue
			}
			if satisfied(file.File.Source, dependency, after) {
				continue
			}
			if changed[archive.Key(file.Path)] || satisfied(file.File.Source, dependency, before) {
				return fmt.Errorf("更新后 %s 的必需依赖无法满足，请调整所选版本", path.Base(file.Path))
			}
		}
	}
	for i, left := range after {
		if left.Enabled {
			for _, right := range after[i+1:] {
				if !right.Enabled || archive.Key(left.Path) == archive.Key(right.Path) {
					continue
				}
				if (changed[archive.Key(left.Path)] || changed[archive.Key(right.Path)]) && Incompatible(left.File, right.File) {
					return errors.New("更新后的资产声明不兼容")
				}
			}
		}
	}
	return nil
}
