package mods

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
)

type parsedVariant struct {
	Variant
	jars []string
}

func fabric(data []byte) (parsedVariant, error) {
	var raw struct {
		SchemaVersion                          int
		ID, Name, Version, Environment         string
		Provides                               []string
		Depends, Recommends, Breaks, Conflicts map[string]Versions
		Jars                                   []struct{ File string }
	}
	result := parsedVariant{Variant: Variant{Loader: "fabric"}}
	if err := json.Unmarshal(data, &raw); err != nil {
		return result, errors.New("Fabric 声明不是有效 JSON")
	}
	if raw.SchemaVersion != 1 {
		return result, errors.New("尚未支持此 Fabric 声明版本")
	}
	mod := Mod{ID: raw.ID, Name: raw.Name, Version: raw.Version, Environment: raw.Environment}
	for _, id := range raw.Provides {
		mod.Provides = append(mod.Provides, Provided{ID: id, Version: raw.Version})
	}
	for _, group := range []struct {
		values                          map[string]Versions
		optional, warning, incompatible bool
	}{
		{raw.Depends, false, false, false}, {raw.Recommends, false, true, false}, {raw.Breaks, false, false, true}, {raw.Conflicts, false, true, true},
	} {
		if len(group.values) > 128 {
			return result, errors.New("Fabric 依赖数量过多")
		}
		keys := make([]string, 0, len(group.values))
		for id := range group.values {
			keys = append(keys, id)
		}
		slices.Sort(keys)
		for _, id := range keys {
			mod.Requires = append(mod.Requires, Requirement{ID: id, Versions: group.values[id], Syntax: Fabric, Optional: group.optional, Warning: group.warning, Incompatible: group.incompatible})
		}
	}
	for _, jar := range raw.Jars {
		result.jars = append(result.jars, jar.File)
	}
	result.Mods = []Mod{mod}
	return result, validateParsed(result)
}

func quilt(data []byte) (parsedVariant, error) {
	var raw struct {
		Schema int `json:"schema_version"`
		Loader struct {
			ID, Group, Version string
			Metadata           struct{ Name string }
			Provides           []json.RawMessage
			Depends, Breaks    []json.RawMessage
			Jars               []string
			Plugins            json.RawMessage
		} `json:"quilt_loader"`
		Minecraft struct{ Environment string }
	}
	result := parsedVariant{Variant: Variant{Loader: "quilt"}}
	if err := json.Unmarshal(data, &raw); err != nil {
		return result, errors.New("Quilt 声明不是受支持的 JSON（JSON5 扩展尚未接入）")
	}
	if raw.Schema != 1 {
		return result, errors.New("尚未支持此 Quilt 声明版本")
	}
	loader := raw.Loader
	if len(loader.Plugins) > 0 && string(bytes.TrimSpace(loader.Plugins)) != "[]" {
		result.Incomplete = true
		result.Problems = append(result.Problems, "自定义 Quilt 加载器插件尚未评估")
	}
	mod := Mod{ID: loader.ID, Group: loader.Group, Name: loader.Metadata.Name, Version: loader.Version, Environment: raw.Minecraft.Environment}
	for _, item := range loader.Provides {
		var id string
		provided := Provided{Version: mod.Version}
		if json.Unmarshal(item, &id) == nil {
			provided.Group, provided.ID = splitID(id)
		} else {
			var object struct{ ID, Version string }
			if err := json.Unmarshal(item, &object); err != nil {
				return result, errors.New("无效 Quilt provides")
			}
			provided.Group, provided.ID = splitID(object.ID)
			if object.Version != "" {
				provided.Version = object.Version
			}
		}
		mod.Provides = append(mod.Provides, provided)
	}
	for _, group := range []struct {
		values       []json.RawMessage
		incompatible bool
	}{{loader.Depends, false}, {loader.Breaks, true}} {
		if len(group.values) > 128 {
			return result, errors.New("Quilt 依赖数量过多")
		}
		for _, raw := range group.values {
			rule, err := quiltRule(raw, group.incompatible, 0)
			if err != nil {
				return result, err
			}
			rule.Incompatible = group.incompatible
			mod.Requires = append(mod.Requires, rule)
		}
	}
	result.Mods, result.jars = []Mod{mod}, loader.Jars
	return result, validateParsed(result)
}

func quiltRule(data []byte, all bool, depth int) (Requirement, error) {
	rule := Requirement{Syntax: Quilt}
	data = bytes.TrimSpace(data)
	if depth > 12 || len(data) == 0 || len(data) > 65536 {
		return rule, errors.New("Quilt 依赖组超出限制")
	}
	var id string
	if json.Unmarshal(data, &id) == nil {
		rule.Group, rule.ID = splitID(id)
		return rule, validateRule(rule, 0)
	}
	if data[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			return rule, err
		}
		if len(items) == 0 || len(items) > 64 {
			return rule, errors.New("Quilt 依赖组数量无效")
		}
		for _, item := range items {
			child, err := quiltRule(item, all, depth+1)
			if err != nil {
				return rule, err
			}
			if all {
				rule.All = append(rule.All, child)
			} else {
				rule.Any = append(rule.Any, child)
			}
		}
		return rule, nil
	}
	var raw struct {
		ID, Environment string
		Versions        Versions
		Optional        bool
		Unless          json.RawMessage
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return rule, errors.New("无效 Quilt 依赖")
	}
	rule.Group, rule.ID = splitID(raw.ID)
	rule.Versions, rule.Optional = raw.Versions, raw.Optional
	if raw.Environment == "server" || raw.Environment == "dedicated_server" {
		rule.ServerOnly = true
	} else if raw.Environment != "" && raw.Environment != "*" && raw.Environment != "client" {
		return rule, errors.New("未知 Quilt 依赖环境")
	}
	if len(raw.Unless) > 0 {
		unless, err := quiltRule(raw.Unless, false, depth+1)
		if err != nil {
			return rule, err
		}
		rule.Unless = &unless
	}
	return rule, validateRule(rule, 0)
}

func validateRule(rule Requirement, depth int) error {
	if depth > 12 {
		return errors.New("依赖条件过深")
	}
	if len(rule.Any) > 0 || len(rule.All) > 0 {
		if len(rule.Any)+len(rule.All) > 64 {
			return errors.New("依赖条件过多")
		}
		for _, group := range [][]Requirement{rule.Any, rule.All} {
			for _, child := range group {
				if err := validateRule(child, depth+1); err != nil {
					return err
				}
			}
		}
	} else if !validID(rule.ID) || rule.Group != "" && !validID(rule.Group) {
		return errors.New("模组依赖 ID 无效")
	}
	if rule.Unless != nil {
		return validateRule(*rule.Unless, depth+1)
	}
	return nil
}

func validateParsed(result parsedVariant) error {
	if len(result.Mods) > 128 || len(result.jars) > 128 {
		return errors.New("声明包含过多模组或内嵌包")
	}
	for _, mod := range result.Mods {
		if !validID(mod.ID) || len(mod.Name) > 512 || len(mod.Version) > 512 || mod.Version == "" || len(mod.Requires) > 256 || len(mod.Provides) > 128 {
			return errors.New("模组声明身份或大小无效")
		}
		if mod.Environment != "" && mod.Environment != "*" && mod.Environment != "client" && mod.Environment != "server" && mod.Environment != "dedicated_server" {
			return errors.New("未知模组环境")
		}
		for _, provided := range mod.Provides {
			if !validID(provided.ID) || len(provided.Version) > 512 || provided.Group != "" && !validID(provided.Group) {
				return errors.New("无效 provides 身份")
			}
		}
		for _, rule := range mod.Requires {
			if err := validateRule(rule, 0); err != nil {
				return err
			}
		}
	}
	return nil
}
