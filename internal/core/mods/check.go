package mods

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type Runtime struct{ GameVersion, Loader, LoaderVersion, JavaVersion string }
type Issue struct {
	Code, Path, PeerPath, ModID, TargetID, Title, Detail string
	Error, CanEnable                                     bool
}
type Report struct {
	Issues             []Issue
	Modules            int
	Truncated          bool
	relations          []relationship
	relationsTruncated bool
	structural         []Issue
}

type relationship struct {
	key, path, peer, detail, reason string
	state                           Verdict
	fatal                           bool
}
type provider struct {
	id, group, version, path          string
	enabled, nested, alias, uncertain bool
}
type evaluation struct {
	verdict              Verdict
	reason, peer, target string
	enable               bool
}

// Check evaluates declarations for the configured client runtime. Optional
// embedded candidates are not proof of the loader's eventual version selection.
func Check(ctx context.Context, files []File, runtime Runtime, incomplete bool) (Report, error) {
	return check(ctx, files, runtime, incomplete, false)
}

func check(ctx context.Context, files []File, runtime Runtime, incomplete, capture bool) (Report, error) {
	r := Report{}
	if len(files) > 3000 {
		return r, errors.New("本地声明输入超出上限")
	}
	add := func(issue Issue) {
		if len(issue.Detail) > 2048 {
			issue.Detail = "声明条件过长，请检查原始模组元数据"
		}
		if capture && issue.Error && issue.Code != "dependency" {
			r.structural = append(r.structural, issue)
		}
		if len(r.Issues) >= 1000 {
			r.Truncated = true
			if issue.Error {
				for i := len(r.Issues) - 1; i >= 0; i-- {
					if !r.Issues[i].Error {
						r.Issues[i] = issue
						break
					}
				}
			}
			return
		}
		r.Issues = append(r.Issues, issue)
	}
	providers := make(map[string][]provider)
	type chosen struct {
		path string
		mod  Mod
	}
	var modules []chosen
	partial := incomplete
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		if file.Problem != "" {
			add(Issue{Code: "read", Path: file.Path, Title: "无法检查模组声明", Detail: file.Problem})
			partial = partial || file.Enabled
			continue
		}
		for _, problem := range file.Archive.Problems {
			add(Issue{Code: "read", Path: file.Path, Title: "部分声明无法解析", Detail: problem})
		}
		var selected *Variant
		for index := range file.Archive.Variants {
			variant := &file.Archive.Variants[index]
			if variant.Loader == runtime.Loader {
				selected = variant
				break
			}
		}
		if selected == nil && runtime.Loader == "quilt" {
			for index := range file.Archive.Variants {
				if file.Archive.Variants[index].Loader == "fabric" {
					selected = &file.Archive.Variants[index]
					break
				}
			}
		}
		if selected == nil {
			if file.Enabled {
				partial = partial || runtime.Loader == "" || len(file.Archive.Variants) == 0 || len(file.Archive.Problems) > 0
				loaders := make([]string, 0, len(file.Archive.Variants))
				for _, variant := range file.Archive.Variants {
					loaders = append(loaders, variant.Loader)
				}
				title := "没有匹配当前加载器的声明"
				if runtime.Loader == "" {
					title = "实例未配置加载器，声明关系待确认"
				}
				add(Issue{Code: "loader", Path: file.Path, Title: title, Detail: "声明：" + strings.Join(loaders, " / ") + "；配置：" + runtime.Loader, Error: runtime.Loader != "" && len(loaders) > 0})
			}
			continue
		}
		if selected.Incomplete && file.Enabled {
			partial = true
		}
		for _, problem := range selected.Problems {
			add(Issue{Code: "nested", Path: file.Path, Title: "内嵌包检查不完整", Detail: problem})
		}
		for _, mod := range selected.Mods {
			if mod.Environment == "server" || mod.Environment == "dedicated_server" {
				if file.Enabled && !mod.Nested {
					add(Issue{Code: "environment", Path: file.Path, ModID: mod.ID, Title: "声明仅用于独立服务端", Detail: mod.ID + " 不作为客户端依赖提供者"})
				}
				continue
			}
			if len(modules) >= 8192 {
				partial, r.Truncated = true, true
				break
			}
			providers[mod.ID] = append(providers[mod.ID], provider{id: mod.ID, group: mod.Group, version: mod.Version, path: file.Path, enabled: file.Enabled, nested: mod.Nested})
			for _, alias := range mod.Provides {
				version := alias.Version
				if version == "" {
					version = mod.Version
				}
				providers[alias.ID] = append(providers[alias.ID], provider{id: alias.ID, group: alias.Group, version: version, path: file.Path, enabled: file.Enabled, nested: mod.Nested, alias: true})
			}
			if file.Enabled {
				modules = append(modules, chosen{file.Path, mod})
			}
		}
	}
	game := runtime.GameVersion
	for _, ch := range game {
		if ch != '.' && (ch < '0' || ch > '9') {
			game = ""
			break
		}
	}
	builtins := map[string]string{"minecraft": game, "java": runtime.JavaVersion}
	switch runtime.Loader {
	case "fabric":
		builtins["fabricloader"] = runtime.LoaderVersion
	case "quilt":
		builtins["quilt_loader"], builtins["fabricloader"] = runtime.LoaderVersion, ""
	case "forge":
		builtins["forge"], builtins["javafml"], builtins["lowcodefml"] = runtime.LoaderVersion, "", ""
	case "neoforge":
		builtins["neoforge"], builtins["javafml"], builtins["lowcodefml"] = runtime.LoaderVersion, "", ""
	}
	for id, version := range builtins {
		providers[id] = []provider{{id: id, version: version, enabled: true, uncertain: version == ""}}
	}
	var evaluate func(Requirement, bool, int) evaluation
	evaluate = func(rule Requirement, negative bool, depth int) evaluation {
		if depth > 16 {
			return evaluation{verdict: Unknown, reason: "条件层数超过上限"}
		}
		if rule.ServerOnly {
			if negative {
				return evaluation{verdict: No}
			}
			return evaluation{verdict: Yes}
		}
		if rule.Unless != nil {
			except := evaluate(*rule.Unless, true, depth+1)
			if except.verdict == Yes {
				if negative {
					return evaluation{verdict: No}
				}
				return evaluation{verdict: Yes}
			}
			if except.verdict == Unknown {
				base := rule
				base.Unless = nil
				resolved := evaluate(base, negative, depth+1)
				if !negative && resolved.verdict == Yes || negative && resolved.verdict == No {
					return resolved
				}
				return evaluation{verdict: Unknown, reason: "unless 条件无法确认"}
			}
		}
		if len(rule.Any) > 0 || len(rule.All) > 0 {
			items, any := rule.All, false
			if len(rule.Any) > 0 {
				items, any = rule.Any, true
			}
			values := make([]Verdict, 0, len(items))
			for _, child := range items {
				values = append(values, evaluate(child, negative, depth+1).verdict)
			}
			return evaluation{verdict: combine(values, any), reason: "复合依赖条件"}
		}
		result := evaluation{verdict: No, reason: "缺少声明依赖", target: rule.ID}
		present, uncertain, candidate := false, false, false
		hasActual := false
		for _, item := range providers[rule.ID] {
			if item.enabled && !item.nested && !item.alias && (rule.Group == "" || rule.Group == item.group) {
				hasActual = true
				break
			}
		}
		for _, item := range providers[rule.ID] {
			if rule.Group != "" && item.group == "" && item.enabled {
				uncertain = true
				continue
			}
			if rule.Group != "" && item.group != rule.Group || hasActual && item.alias {
				continue
			}
			match := Match(item.version, rule.Versions, rule.Syntax)
			if !item.enabled {
				if match == Yes && !item.nested {
					result.peer, result.enable = item.path, true
				}
				continue
			}
			present = true
			if item.uncertain || match == Unknown {
				uncertain = true
				continue
			}
			if match == Yes {
				if item.nested {
					candidate = true
					continue
				}
				return evaluation{verdict: Yes, target: rule.ID, peer: item.path}
			}
			if result.peer == "" {
				result.peer = item.path
			}
		}
		if !negative && rule.Optional && !present {
			return evaluation{verdict: Yes}
		}
		if uncertain || candidate || partial {
			return evaluation{verdict: Unknown, target: rule.ID, reason: "版本、内嵌候选或未完整读取的模组尚无法确认"}
		}
		if present {
			result.reason, result.enable = "版本范围不满足", false
		} else if !negative && result.enable {
			result.reason = "依赖已停用"
		}
		return result
	}
	actual := make(map[string]provider)
	for _, chosen := range modules {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		mod := chosen.mod
		r.Modules++
		if !mod.Nested {
			if previous, exists := actual[mod.ID]; exists {
				add(Issue{Code: "duplicate", Path: chosen.path, PeerPath: previous.path, ModID: mod.ID, Title: "重复的模组 ID", Detail: mod.ID + " 同时由多个顶层声明提供", Error: true})
			} else {
				actual[mod.ID] = provider{path: chosen.path}
			}
		}
		for _, rule := range mod.Requires {
			if err := ctx.Err(); err != nil {
				return r, err
			}
			result := evaluate(rule, rule.Incompatible, 0)
			if capture {
				if len(r.relations) >= 131072 {
					r.relationsTruncated = true
				} else {
					data, err := json.Marshal(rule)
					if err != nil {
						return r, errors.New("模组条件无法形成稳定身份")
					}
					digest := sha256.Sum256(data)
					state := result.verdict
					if rule.Incompatible {
						if state == Yes {
							state = No
						} else if state == No {
							state = Yes
						}
					}
					detail := mod.ID + " → " + describe(rule)
					if len(detail) > 1024 {
						detail = mod.ID + " 的复合依赖条件"
					}
					reason := result.reason
					if rule.Incompatible {
						reason = "声明不兼容"
					}
					r.relations = append(r.relations, relationship{key: mod.Group + ":" + mod.ID + ":" + hex.EncodeToString(digest[:]), path: chosen.path, peer: result.peer, detail: detail, reason: reason, state: state, fatal: !rule.Warning && !mod.Nested})
				}
			}
			if !rule.Incompatible && result.verdict == Yes || rule.Incompatible && result.verdict == No {
				continue
			}
			fatal := result.verdict != Unknown && !rule.Warning && !mod.Nested
			title := result.reason
			if result.verdict == Unknown {
				title = "声明关系无法确认"
			} else if rule.Incompatible {
				title = "声明不兼容的模组同时存在"
			}
			search := result.target
			if _, builtin := builtins[search]; builtin {
				search = ""
			}
			detail := mod.ID + " " + mod.Version + " → " + describe(rule)
			if mod.Location != "" {
				detail += " · 内嵌 " + mod.Location
			}
			add(Issue{Code: "dependency", Path: chosen.path, PeerPath: result.peer, ModID: mod.ID, TargetID: search, Title: title, Detail: detail, Error: fatal, CanEnable: result.enable && !mod.Nested && result.verdict == No && !rule.Incompatible})
		}
	}
	slices.SortStableFunc(r.Issues, func(a, b Issue) int {
		if a.Error != b.Error {
			if a.Error {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Path+"\x00"+a.ModID+"\x00"+a.Detail, b.Path+"\x00"+b.ModID+"\x00"+b.Detail)
	})
	return r, nil
}

func describe(rule Requirement) string {
	if len(rule.Any) > 0 || len(rule.All) > 0 {
		items, separator := rule.All, " 且 "
		if len(rule.Any) > 0 {
			items, separator = rule.Any, " 或 "
		}
		values := make([]string, 0, len(items))
		for _, item := range items {
			values = append(values, describe(item))
		}
		return "(" + strings.Join(values, separator) + ")"
	}
	id := rule.ID
	if rule.Group != "" {
		id = rule.Group + ":" + id
	}
	return fmt.Sprintf("%s %s", id, rule.Versions.String())
}
