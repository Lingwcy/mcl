package meta

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

type Platform struct {
	OS       string
	Arch     string
	Version  string
	Features map[string]bool
}

func CurrentPlatform() Platform {
	os := runtime.GOOS
	if os == "darwin" {
		os = "osx"
	}
	return Platform{OS: os, Arch: NormalizeArch(runtime.GOARCH)}
}

func NormalizeArch(arch string) string {
	switch strings.ToLower(arch) {
	case "amd64", "x86_64", "x64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "x86", "i386", "i686", "386":
		return "386"
	default:
		return arch
	}
}

type Rule struct {
	Action string `json:"action"`
	OS     *struct {
		Name    string `json:"name"`
		Arch    string `json:"arch"`
		Version string `json:"version"`
	} `json:"os,omitempty"`
	Features map[string]bool `json:"features,omitempty"`
}

// UnmarshalJSON rejects unknown rule conditions: ignoring one could turn a
// future restriction into an unconditional allow on the wrong platform.
func (r *Rule) UnmarshalJSON(data []byte) error {
	type plain Rule
	var decoded plain
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&decoded); err != nil {
		return fmt.Errorf("unsupported rule: %w", err)
	}
	*r = Rule(decoded)
	return ValidateRules([]Rule{*r})
}

func ValidateRules(rules []Rule) error {
	for _, rule := range rules {
		if rule.Action != "allow" && rule.Action != "disallow" {
			return fmt.Errorf("unsupported rule action %q", rule.Action)
		}
		if rule.OS != nil && rule.OS.Version != "" {
			if _, err := regexp.Compile(rule.OS.Version); err != nil {
				return fmt.Errorf("unsupported OS version expression: %w", err)
			}
		}
	}
	return nil
}

// Allowed evaluates in document order; the last matching rule wins.
func Allowed(rules []Rule, platform Platform) (bool, error) {
	if err := ValidateRules(rules); err != nil {
		return false, err
	}
	allowed := len(rules) == 0
	for _, rule := range rules {
		matches, err := rule.matches(platform)
		if err != nil {
			return false, err
		}
		if matches {
			allowed = rule.Action == "allow"
		}
	}
	return allowed, nil
}

func (r Rule) matches(p Platform) (bool, error) {
	if r.OS != nil {
		if r.OS.Name != "" && r.OS.Name != p.OS {
			return false, nil
		}
		if r.OS.Arch != "" && NormalizeArch(r.OS.Arch) != NormalizeArch(p.Arch) {
			return false, nil
		}
		if r.OS.Version != "" {
			if p.Version == "" {
				return false, errors.New("OS version is required to evaluate this rule")
			}
			matched, err := regexp.MatchString(r.OS.Version, p.Version)
			if err != nil || !matched {
				return false, err
			}
		}
	}
	for name, expected := range r.Features {
		if p.Features[name] != expected {
			return false, nil
		}
	}
	return true, nil
}

type Argument struct {
	Values []string
	Rules  []Rule
}

// MarshalJSON preserves the official string/conditional representation. This
// also makes cached, composed loader profiles readable by UnmarshalJSON.
func (a Argument) MarshalJSON() ([]byte, error) {
	if len(a.Rules) == 0 && len(a.Values) == 1 {
		return json.Marshal(a.Values[0])
	}
	return json.Marshal(struct {
		Rules []Rule   `json:"rules,omitempty"`
		Value []string `json:"value"`
	}{a.Rules, a.Values})
}

func (a *Argument) UnmarshalJSON(data []byte) error {
	var value string
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		a.Values, a.Rules = []string{value}, nil
		return nil
	}
	var conditional struct {
		Rules []Rule          `json:"rules"`
		Value json.RawMessage `json:"value"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&conditional); err != nil {
		return fmt.Errorf("parse conditional argument: %w", err)
	}
	conditional.Value = bytes.TrimSpace(conditional.Value)
	if len(conditional.Value) > 0 && conditional.Value[0] == '"' {
		if err := json.Unmarshal(conditional.Value, &value); err != nil {
			return err
		}
		a.Values = []string{value}
	} else if err := json.Unmarshal(conditional.Value, &a.Values); err != nil || len(a.Values) == 0 {
		return errors.New("argument value must be a string or a nonempty string array")
	}
	a.Rules = conditional.Rules
	return nil
}

func SelectArguments(arguments []Argument, platform Platform) ([]string, error) {
	var out []string
	for _, argument := range arguments {
		allowed, err := Allowed(argument.Rules, platform)
		if err != nil {
			return nil, err
		}
		if allowed {
			out = append(out, argument.Values...)
		}
	}
	return out, nil
}
