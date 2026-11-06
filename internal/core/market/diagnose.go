package market

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"mcl/internal/core/archive"
)

type AssetEvidence struct {
	Path                        string
	Kind                        Kind
	Size                        int64
	SHA1                        string
	Enabled, Directory, Checked bool
}

type DiagnosticInput struct {
	GameVersion, Loader string
	Files               []AssetEvidence
	Records             []InstallFile
	Truncated           bool
}

type IssueCode string

const (
	IssueUnknown               IssueCode = "unknown"
	IssueUnchecked             IssueCode = "unchecked"
	IssueChanged               IssueCode = "changed"
	IssueMissingFile           IssueCode = "missing-file"
	IssueMovedFile             IssueCode = "moved-file"
	IssueDuplicateFile         IssueCode = "duplicate-file"
	IssueDuplicateProject      IssueCode = "duplicate-project"
	IssueMissingMetadata       IssueCode = "missing-metadata"
	IssueGameVersion           IssueCode = "game-version"
	IssueLoader                IssueCode = "loader"
	IssueMissingDependency     IssueCode = "missing-dependency"
	IssueUnconfirmedDependency IssueCode = "unconfirmed-dependency"
	IssueDisabledDependency    IssueCode = "disabled-dependency"
	IssueDependencyVersion     IssueCode = "dependency-version"
	IssueInvalidDependency     IssueCode = "invalid-dependency"
	IssueConflict              IssueCode = "conflict"
	IssueCycle                 IssueCode = "cycle"
	IssueLocalDeclaration      IssueCode = "local-declaration"
)

type AssetIssue struct {
	ID                            string
	Code                          IssueCode
	Error                         bool
	Path, PeerPath, Title, Detail string
	Origin                        *InstallFile
	Dependency                    Dependency
	LocalID                       string
	CanEnable                     bool
}

type DiagnosticReport struct {
	Issues                                       []AssetIssue
	Checked, Unknown, Unchecked, Missing, Errors int
	Unconfirmed                                  int
	Truncated                                    bool
}

// Diagnose uses only verified local evidence and saved catalog declarations.
// It does not infer project identity from filenames, slugs or loader mod IDs.
func Diagnose(ctx context.Context, input DiagnosticInput) (DiagnosticReport, error) {
	r := DiagnosticReport{Truncated: input.Truncated}
	if len(input.Files) > 3000 || len(input.Records) > 6000 {
		return r, errors.New("资产诊断输入超出上限")
	}
	seenIssues := make(map[string]bool)
	add := func(issue AssetIssue) {
		issue.Detail = shortText(issue.Detail, 2048)
		id := string(issue.Code) + "\x00" + issue.Path + "\x00" + issue.PeerPath + "\x00" + issue.Detail
		if seenIssues[id] {
			return
		}
		issue.ID = id
		if len(r.Issues) >= 1000 {
			r.Truncated = true
			// Keep actual errors visible even when unknown-file notices filled
			// the report first. The identity set remains bounded by the report.
			if issue.Error && r.Errors < 1000 {
				for index := len(r.Issues) - 1; index >= 0; index-- {
					if !r.Issues[index].Error {
						delete(seenIssues, r.Issues[index].ID)
						r.Issues[index] = issue
						seenIssues[id] = true
						r.Errors++
						break
					}
				}
			}
			return
		}
		seenIssues[id] = true
		r.Issues = append(r.Issues, issue)
		if issue.Error {
			r.Errors++
		}
	}
	canonical := func(name string) string { return archive.Key(strings.TrimSuffix(name, ".disabled")) }
	scopeKey := func(source Source, directory, id string) string {
		return string(source) + "\x00" + archive.Key(directory) + "\x00" + id
	}
	partialScopes := make(map[string]bool)
	orderedRecords := slices.Clone(input.Records)
	slices.SortStableFunc(orderedRecords, func(a, b InstallFile) int {
		if a.Source != b.Source {
			if a.Source == Modrinth {
				return -1
			}
			return 1
		}
		return 0
	})
	records := make(map[string][]InstallFile)
	for _, file := range orderedRecords {
		if err := file.Metadata.Validate(); err != nil {
			return r, err
		}
		records[canonical(file.Path)] = append(records[canonical(file.Path)], file)
	}
	present := make(map[string][]AssetEvidence)
	byHash := make(map[string][]AssetEvidence)
	actualPaths := make(map[string]string)
	var known []InstalledFile
	for _, file := range input.Files {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		key := canonical(file.Path)
		actualPaths[key] = file.Path
		present[key] = append(present[key], file)
		if len(present[key]) > 1 {
			add(AssetIssue{Code: IssueDuplicateFile, Error: true, Path: file.Path, PeerPath: present[key][0].Path, Title: "同名资产并存", Detail: "文件名、大小写或启用/停用别名冲突，请保留需要的一份"})
		}
		if !file.Checked {
			if file.Enabled {
				for _, source := range []Source{Modrinth, CurseForge} {
					partialScopes[scopeKey(source, path.Dir(file.Path), "")] = true
				}
			}
			r.Unchecked++
			add(AssetIssue{Code: IssueUnchecked, Path: file.Path, Title: "未校验", Detail: "目录型包、超出边界或读取失败的文件不作为依赖证据"})
			continue
		}
		r.Checked++
		hashKey := strings.ToLower(file.SHA1)
		byHash[hashKey] = append(byHash[hashKey], file)
		matched := false
		matchedSources := make(map[Source]bool)
		for _, origin := range records[key] {
			if origin.File.Size == file.Size && strings.EqualFold(origin.File.SHA1, file.SHA1) {
				matched = true
				matchedSources[origin.Source] = true
				known = append(known, InstalledFile{File: origin, Path: file.Path, Enabled: file.Enabled})
			}
		}
		if file.Enabled {
			for _, source := range []Source{Modrinth, CurseForge} {
				if !matchedSources[source] {
					partialScopes[scopeKey(source, path.Dir(file.Path), "")] = true
				}
			}
		}
		if !matched {
			r.Unknown++
			code, title, detail := IssueUnknown, "未识别来源", "可通过工具栏按摘要识别；此文件的依赖尚未检查"
			if len(records[key]) > 0 {
				code, title, detail = IssueChanged, "文件与来源记录不符", "大小或摘要已改变，旧来源不能证明当前文件身份；请重新识别"
			}
			add(AssetIssue{Code: code, Error: code == IssueChanged, Path: file.Path, Title: title, Detail: detail})
		}
	}
	missingSeen := make(map[string]bool)
	for _, origin := range orderedRecords {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		key := canonical(origin.Path)
		if input.Truncated || len(present[key]) > 0 || missingSeen[key] {
			continue
		}
		missingSeen[key] = true
		var peer string
		for _, file := range byHash[strings.ToLower(origin.File.SHA1)] {
			if path.Dir(strings.TrimSuffix(file.Path, ".disabled")) == path.Dir(origin.Path) && file.Size == origin.File.Size {
				peer = file.Path
				break
			}
		}
		copy := origin
		if peer != "" {
			add(AssetIssue{Code: IssueMovedFile, Path: origin.Path, PeerPath: peer, Title: "来源路径已改变", Detail: "同目录存在摘要相同的文件，请重新识别以保存当前路径", Origin: &copy})
		} else {
			r.Missing++
			add(AssetIssue{Code: IssueMissingFile, Path: origin.Path, Title: "已记录的文件缺失", Detail: "可查看原版本并确认重新安装；也可能是手动移除后留下的来源记录", Origin: &copy})
		}
	}
	projects, versions := make(map[string][]InstalledFile), make(map[string][]InstalledFile)
	firstActive := make(map[string]InstalledFile)
	for _, file := range known {
		directory := path.Dir(file.Path)
		key := scopeKey(file.File.Source, directory, file.File.ProjectID)
		projects[key] = append(projects[key], file)
		if file.Enabled {
			if _, found := firstActive[key]; !found {
				firstActive[key] = file
			}
		}
		key = scopeKey(file.File.Source, directory, file.File.VersionID)
		versions[key] = append(versions[key], file)
	}
	// Walk input order for stable reports; maps are used only for lookups.
	graph := make(map[string][]string)
	var graphOrder []string
	graphSeen := make(map[string]bool)
	for _, file := range known {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		if !file.Enabled {
			continue
		}
		origin := file.File
		copy := origin
		key := canonical(file.Path)
		if !graphSeen[key] {
			graphSeen[key] = true
			graphOrder = append(graphOrder, key)
		}
		baseIssue := AssetIssue{Path: file.Path, Origin: &copy}
		push := func(code IssueCode, fatal bool, title, detail, peer string, dependency Dependency) {
			issue := baseIssue
			issue.Code, issue.Error, issue.Title, issue.Detail, issue.PeerPath, issue.Dependency = code, fatal, title, detail, peer, dependency
			add(issue)
		}
		if other, found := firstActive[scopeKey(origin.Source, path.Dir(file.Path), origin.ProjectID)]; found {
			if canonical(other.Path) != key {
				push(IssueDuplicateProject, true, "同一项目有多份启用文件", string(origin.Source)+" · "+origin.ProjectID, other.Path, Dependency{})
			}
		}
		metadata := origin.Metadata
		if metadata == nil {
			push(IssueMissingMetadata, false, "来源缺少依赖资料", "请重新识别，当前记录无法用于检查依赖", "", Dependency{})
			continue
		}
		if len(metadata.GameVersions) > 0 && !slices.Contains(metadata.GameVersions, input.GameVersion) {
			push(IssueGameVersion, true, "游戏版本不匹配", "实例 "+input.GameVersion+"；来源声明 "+strings.Join(metadata.GameVersions, ", "), "", Dependency{})
		}
		if path.Dir(file.Path) == "mods" && len(metadata.Loaders) > 0 {
			if input.Loader == "" {
				push(IssueLoader, false, "实例未配置加载器", "文件存在不代表能够加载；来源声明 "+strings.Join(metadata.Loaders, " / "), "", Dependency{})
			} else if !slices.Contains(metadata.Loaders, input.Loader) {
				push(IssueLoader, true, "加载器不匹配", "实例 "+input.Loader+"；来源声明 "+strings.Join(metadata.Loaders, " / "), "", Dependency{})
			}
		}
		for _, dependency := range metadata.Dependencies {
			if err := ctx.Err(); err != nil {
				return r, err
			}
			if !dependency.Required && !dependency.Incompatible {
				continue
			}
			if dependency.ProjectID == "" && dependency.VersionID == "" {
				push(IssueInvalidDependency, false, "依赖标识无法解析", "来源未提供项目或版本标识，不能自动安装", "", dependency)
				continue
			}
			var candidates []InstalledFile
			if dependency.VersionID != "" {
				candidates = versions[scopeKey(origin.Source, path.Dir(file.Path), dependency.VersionID)]
			} else {
				candidates = projects[scopeKey(origin.Source, path.Dir(file.Path), dependency.ProjectID)]
			}
			var active, disabled, wrong string
			for _, candidate := range candidates {
				if DependencyMatches(origin.Source, dependency, candidate.File) {
					if candidate.Enabled {
						active = candidate.Path
						break
					} else {
						disabled = candidate.Path
					}
				} else if candidate.Enabled {
					wrong = candidate.Path
				}
			}
			if active == "" && dependency.ProjectID != "" && dependency.VersionID != "" {
				if candidate, found := firstActive[scopeKey(origin.Source, path.Dir(file.Path), dependency.ProjectID)]; found && candidate.File.VersionID != dependency.VersionID {
					wrong = candidate.Path
				}
			}
			label := string(origin.Source) + " · " + dependency.ProjectID
			if dependency.VersionID != "" {
				label += " · 版本 " + dependency.VersionID
			}
			if dependency.Incompatible {
				if active != "" {
					push(IssueConflict, true, "存在声明不兼容的资产", label, active, dependency)
				}
				continue
			}
			if active != "" {
				graph[key] = append(graph[key], canonical(active))
				continue
			}
			if input.Truncated || partialScopes[scopeKey(origin.Source, path.Dir(file.Path), "")] {
				r.Unconfirmed++
				peer := disabled
				if peer == "" {
					peer = wrong
				}
				push(IssueUnconfirmedDependency, false, "无法确认必需依赖", label+"；存在未识别、未校验、仅其他来源身份或未读取的文件，请补齐识别后重查", peer, dependency)
				continue
			}
			if disabled != "" {
				push(IssueDisabledDependency, true, "必需依赖已停用", label, disabled, dependency)
			} else if wrong != "" {
				push(IssueDependencyVersion, true, "必需依赖版本不符", label, wrong, dependency)
			} else {
				push(IssueMissingDependency, true, "缺少已确认的必需依赖", label+"；未识别或未校验的文件不计入", "", dependency)
			}
		}
	}
	color := make(map[string]uint8)
	var walk func(string, int) error
	walk = func(node string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 128 {
			r.Truncated = true
			return nil
		}
		color[node] = 1
		for _, next := range graph[node] {
			if color[next] == 1 {
				add(AssetIssue{Code: IssueCycle, Error: true, Path: actualPaths[node], PeerPath: actualPaths[next], Title: "存在必需依赖循环", Detail: "来源声明互相依赖，需要核对项目版本"})
			} else if color[next] == 0 {
				if err := walk(next, depth+1); err != nil {
					return err
				}
			}
		}
		color[node] = 2
		return nil
	}
	for _, key := range graphOrder {
		if color[key] == 0 {
			if err := walk(key, 0); err != nil {
				return r, err
			}
		}
	}
	slices.SortStableFunc(r.Issues, func(a, b AssetIssue) int {
		if a.Error != b.Error {
			if a.Error {
				return -1
			}
			return 1
		}
		if compare := strings.Compare(a.Path, b.Path); compare != 0 {
			return compare
		}
		return strings.Compare(a.ID, b.ID)
	})
	return r, nil
}

func (i AssetIssue) Summary() string { return fmt.Sprintf("%s · %s", path.Base(i.Path), i.Title) }
