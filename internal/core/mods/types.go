// Package mods inspects declarations without executing or extracting mod code.
package mods

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

type Verdict uint8

const (
	Unknown Verdict = iota
	No
	Yes
)

type Syntax string

const (
	Fabric Syntax = "fabric"
	Quilt  Syntax = "quilt"
	Maven  Syntax = "maven"
)

type Versions struct {
	Text     string
	Any, All []Versions
}

func (v *Versions) UnmarshalJSON(data []byte) error { return v.decode(data, 0) }
func (v *Versions) decode(data []byte, depth int) error {
	data = bytes.TrimSpace(data)
	if depth > 12 || len(data) > 32768 {
		return errors.New("版本条件过深或过大")
	}
	if len(data) == 0 {
		return errors.New("版本条件为空")
	}
	switch data[0] {
	case '"':
		if err := json.Unmarshal(data, &v.Text); err != nil {
			return err
		}
		if len(v.Text) > 512 {
			return errors.New("版本表达式过长")
		}
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		if len(items) == 0 || len(items) > 64 {
			return errors.New("版本条件数量无效")
		}
		for _, item := range items {
			var child Versions
			if err := child.decode(item, depth+1); err != nil {
				return err
			}
			v.Any = append(v.Any, child)
		}
	case '{':
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err != nil {
			return err
		}
		if len(object) != 1 {
			return errors.New("版本组必须使用单一 any/all")
		}
		for key, raw := range object {
			if key != "any" && key != "all" {
				return errors.New("未知版本条件组")
			}
			var items []json.RawMessage
			if err := json.Unmarshal(raw, &items); err != nil {
				return err
			}
			if len(items) == 0 || len(items) > 64 {
				return errors.New("版本条件数量无效")
			}
			for _, item := range items {
				var child Versions
				if err := child.decode(item, depth+1); err != nil {
					return err
				}
				if key == "any" {
					v.Any = append(v.Any, child)
				} else {
					v.All = append(v.All, child)
				}
			}
		}
	default:
		return errors.New("无效版本条件")
	}
	return nil
}

func (v Versions) String() string {
	if len(v.Any) > 0 {
		parts := make([]string, 0, len(v.Any))
		for _, item := range v.Any {
			parts = append(parts, item.String())
		}
		return "(" + strings.Join(parts, " 或 ") + ")"
	}
	if len(v.All) > 0 {
		parts := make([]string, 0, len(v.All))
		for _, item := range v.All {
			parts = append(parts, item.String())
		}
		return "(" + strings.Join(parts, " 且 ") + ")"
	}
	if v.Text == "" {
		return "*"
	}
	return v.Text
}

type Requirement struct {
	ID, Group                                   string
	Versions                                    Versions
	Syntax                                      Syntax
	Optional, Warning, Incompatible, ServerOnly bool
	Any, All                                    []Requirement
	Unless                                      *Requirement
}

type Provided struct{ ID, Group, Version string }
type Mod struct {
	ID, Group, Name, Version, Environment, Location string
	Provides                                        []Provided
	Requires                                        []Requirement
	Nested                                          bool
}
type Variant struct {
	Loader     string
	Mods       []Mod
	Problems   []string
	Incomplete bool
}
type Archive struct {
	Variants  []Variant
	Problems  []string
	ReadBytes int64
}
type File struct {
	Path    string
	Enabled bool
	Archive Archive
	Problem string
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > 100 {
		return false
	}
	for _, char := range id {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-' || char == '.') {
			return false
		}
	}
	return true
}

func splitID(id string) (string, string) {
	if group, name, ok := strings.Cut(id, ":"); ok {
		return group, name
	}
	return "", id
}
