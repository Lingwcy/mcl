package java

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"mcl/internal/core/meta"
)

const MaxConstraints = 16

type Bound struct {
	Version   Version
	Inclusive bool
}
type Constraint struct {
	Source           string
	Majors           []int
	Minimum, Maximum *Bound
	VMs              []string
	NeedJDK          bool
}

// Constraints contain established requirements only. Unverified compatibility
// suggestions are diagnostic hints and must not be intersected into this set.
type Requirement struct {
	Purpose     string
	Target      meta.Platform
	Constraints []Constraint
}

func GameRequirement(major int, target meta.Platform) Requirement {
	return Requirement{Purpose: "game", Target: target, Constraints: []Constraint{{Source: "official-game", Minimum: &Bound{Version: majorVersion(major), Inclusive: true}}, {Source: "game-major-policy", Majors: []int{major}}}}
}
func ProcessorRequirement(minimum int, target meta.Platform) Requirement {
	var majors []int
	if minimum == 8 {
		majors = []int{8}
	} else {
		for major := max(9, minimum); major <= 25; major++ {
			majors = append(majors, major)
		}
	}
	if majors == nil {
		majors = []int{}
	}
	return Requirement{Purpose: "processor", Target: target, Constraints: []Constraint{{Source: "processor-minimum", Minimum: &Bound{Version: majorVersion(minimum), Inclusive: true}}, {Source: "tool-major-policy", Majors: majors}}}
}
func majorVersion(major int) Version {
	var v Version
	if major > 0 && major <= 100 {
		v.Numbers[0] = uint32(major)
	}
	return v
}
func (r Requirement) Clone() Requirement {
	r.Target.Features = maps.Clone(r.Target.Features)
	r.Constraints = slices.Clone(r.Constraints)
	for i, c := range r.Constraints {
		c.Majors = slices.Clone(c.Majors)
		c.VMs = slices.Clone(c.VMs)
		if c.Minimum != nil {
			v := *c.Minimum
			c.Minimum = &v
		}
		if c.Maximum != nil {
			v := *c.Maximum
			c.Maximum = &v
		}
		r.Constraints[i] = c
	}
	return r
}
func (r Requirement) Intersect(next Requirement) (Requirement, error) {
	if err := r.Validate(); err != nil {
		return Requirement{}, err
	}
	if err := next.Validate(); err != nil {
		return Requirement{}, err
	}
	if r.Purpose != next.Purpose || r.Target.OS != next.Target.OS || meta.NormalizeArch(r.Target.Arch) != meta.NormalizeArch(next.Target.Arch) {
		return Requirement{}, requirementError("policy", "用途或目标平台不同，不能合并Java需求")
	}
	out := r.Clone()
	for _, candidate := range next.Clone().Constraints {
		duplicate := false
		for _, existing := range out.Constraints {
			if sameConstraint(existing, candidate) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out.Constraints = append(out.Constraints, candidate)
		}
	}
	return out, out.Validate()
}
func sameConstraint(a, b Constraint) bool {
	if a.Source != b.Source || a.NeedJDK != b.NeedJDK || !slices.Equal(a.Majors, b.Majors) || !slices.Equal(a.VMs, b.VMs) {
		return false
	}
	sameBound := func(a, b *Bound) bool {
		if a == nil || b == nil {
			return a == nil && b == nil
		}
		return *a == *b
	}
	return sameBound(a.Minimum, b.Minimum) && sameBound(a.Maximum, b.Maximum)
}

func (r Requirement) Validate() error {
	if r.Purpose != "game" && r.Purpose != "processor" {
		return requirementError("policy", "Java用途无效")
	}
	arch := meta.NormalizeArch(r.Target.Arch)
	if arch != "amd64" && arch != "arm64" && arch != "386" {
		return requirementError("policy", "Java目标架构无效")
	}
	if r.Target.OS != "" && r.Target.OS != "osx" && r.Target.OS != "windows" && r.Target.OS != "linux" {
		return requirementError("policy", "Java目标系统无效")
	}
	if len(r.Constraints) == 0 || len(r.Constraints) > MaxConstraints {
		return requirementError("policy", "Java需求数量无效")
	}
	for _, c := range r.Constraints {
		switch c.Source {
		case "official-game", "game-major-policy", "loader-game", "processor-minimum", "official-tool-baseline", "tool-major-policy", "loader-tool":
		default:
			return requirementError("policy", "Java需求来源无效")
		}
		if (c.Source == "official-game" || c.Source == "game-major-policy" || c.Source == "loader-game") && r.Purpose != "game" {
			return requirementError(c.Source, "游戏需求不能用于安装工具")
		}
		if (c.Source == "processor-minimum" || c.Source == "official-tool-baseline" || c.Source == "tool-major-policy" || c.Source == "loader-tool") && r.Purpose != "processor" {
			return requirementError(c.Source, "工具需求不能用于游戏")
		}
		if c.Majors != nil && (len(c.Majors) == 0 || len(c.Majors) > 93) {
			return requirementError(c.Source, "Java允许集合为空或越界")
		}
		seen := map[int]bool{}
		for _, m := range c.Majors {
			if m < 8 || m > 100 || seen[m] {
				return requirementError(c.Source, "Java主版本集合无效")
			}
			seen[m] = true
		}
		for _, bound := range []*Bound{c.Minimum, c.Maximum} {
			if bound != nil && (bound.Version.Major() < 8 || bound.Version.Major() > 100 || !bound.Version.Stable() || !bound.Version.BuildKnown && bound.Version.Build != 0) {
				return requirementError(c.Source, "Java版本区间无效或超出支持范围")
			}
		}
		if c.VMs != nil && (len(c.VMs) == 0 || len(c.VMs) > 2) {
			return requirementError(c.Source, "Java VM允许集合为空或越界")
		}
		seenVM := map[string]bool{}
		for _, vm := range c.VMs {
			if vm != "hotspot" && vm != "openj9" || seenVM[vm] {
				return requirementError(c.Source, "Java VM集合无效")
			}
			seenVM[vm] = true
		}
	}
	if len(r.allowedMajors()) == 0 {
		return requirementError("policy", "Java版本需求互相冲突")
	}
	for _, vm := range []string{"hotspot", "openj9"} {
		allowed := true
		for _, c := range r.Constraints {
			if c.VMs != nil && !slices.Contains(c.VMs, vm) {
				allowed = false
				break
			}
		}
		if allowed {
			return nil
		}
	}
	return requirementError("policy", "Java VM需求互相冲突")
}
func (r Requirement) allowedMajors() []int {
	var majors []int
	for major := 8; major <= 100; major++ {
		low := versionPoint(majorVersion(major))
		next := Version{}
		next.Numbers[0] = uint32(major + 1)
		high := versionPoint(next)
		high = predecessor(high)
		allowed := true
		for _, c := range r.Constraints {
			if c.Majors != nil && !slices.Contains(c.Majors, major) {
				allowed = false
				break
			}
			if c.Minimum != nil {
				point := lowerPoint(*c.Minimum)
				if slices.Compare(point[:], low[:]) > 0 {
					low = point
				}
			}
			if c.Maximum != nil {
				point := upperPoint(*c.Maximum)
				if slices.Compare(point[:], high[:]) < 0 {
					high = point
				}
			}
		}
		if allowed && slices.Compare(low[:], high[:]) <= 0 {
			majors = append(majors, major)
		}
	}
	return majors
}

// Points include all eight release components plus the observed Java build.
// Whole-release exclusive bounds skip every build of that release.
type releasePoint [9]uint64

func versionPoint(v Version) releasePoint {
	var p releasePoint
	for i, n := range v.Numbers {
		p[i] = uint64(n)
	}
	p[8] = uint64(v.Build)
	return p
}
func successor(p releasePoint) releasePoint {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] < 1<<32-1 {
			p[i]++
			return p
		}
		p[i] = 0
	}
	return p
}
func predecessor(p releasePoint) releasePoint {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] > 0 {
			p[i]--
			return p
		}
		p[i] = 1<<32 - 1
	}
	return p
}
func lowerPoint(b Bound) releasePoint {
	p := versionPoint(b.Version)
	if !b.Version.BuildKnown {
		p[8] = 0
		if !b.Inclusive {
			p[8] = 1<<32 - 1
		}
	}
	if !b.Inclusive {
		p = successor(p)
	}
	return p
}
func upperPoint(b Bound) releasePoint {
	p := versionPoint(b.Version)
	if !b.Version.BuildKnown {
		p[8] = 1<<32 - 1
		if !b.Inclusive {
			p[8] = 0
		}
	}
	if !b.Inclusive {
		p = predecessor(p)
	}
	return p
}
func (r Requirement) NeedsJDK() bool {
	for _, c := range r.Constraints {
		if c.NeedJDK {
			return true
		}
	}
	return false
}
func (r Requirement) DownloadMajors() ([]int, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	majors := r.allowedMajors()
	if r.Purpose == "game" {
		if len(majors) > 3 {
			return nil, requirementError("policy", "自动下载需要固定至不超过三个Java主版本")
		}
		return majors, nil
	}
	return slices.DeleteFunc(majors, func(m int) bool { return m != 8 && m != 17 && m != 21 && m != 25 }), nil
}
func (r Requirement) checkVersion(v Version, vm string, compiler bool) error {
	if !v.Stable() {
		return requirementError("policy", "需要稳定版Java")
	}
	for _, c := range r.Constraints {
		if c.Majors != nil && !slices.Contains(c.Majors, v.Major()) {
			return requirementError(c.Source, "Java主版本不在允许集合中")
		}
		if c.NeedJDK && !compiler {
			return requirementError(c.Source, "需要同一Java目录中的JDK编译器文件")
		}
		if c.VMs != nil && !slices.Contains(c.VMs, vm) {
			return requirementError(c.Source, "Java VM不在允许集合中或证据未知")
		}
		for _, bound := range []*Bound{c.Minimum, c.Maximum} {
			if bound != nil && v.CompareRelease(bound.Version) == 0 && bound.Version.BuildKnown && !v.BuildKnown {
				return requirementError(c.Source, "缺少所需Java build证据")
			}
		}
		p := versionPoint(v)
		if c.Minimum != nil {
			lo := lowerPoint(*c.Minimum)
			if slices.Compare(p[:], lo[:]) < 0 {
				return requirementError(c.Source, "Java版本低于需求区间")
			}
		}
		if c.Maximum != nil {
			hi := upperPoint(*c.Maximum)
			if slices.Compare(p[:], hi[:]) > 0 {
				return requirementError(c.Source, "Java版本高于需求区间")
			}
		}
	}
	return nil
}
func (r Requirement) CheckRuntime(runtime Runtime) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := runtime.Check(8, r.Target); err != nil {
		return requirementError("policy", err.Error())
	}
	v, err := runtime.NormalizedVersion()
	if err != nil {
		return err
	}
	return r.checkVersion(v, runtime.VM, runtime.Compiler == "present")
}
func (r Requirement) CheckPackage(p Package) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Arch != meta.NormalizeArch(r.Target.Arch) || r.Target.OS != "" && p.OS != r.Target.OS {
		return requirementError("policy", "Java包平台与需求不同")
	}
	v, err := p.ReleaseVersion()
	if err != nil {
		return err
	}
	return r.checkVersion(v, p.VM, p.Kind == "jdk")
}

type RequirementError struct{ Source, Reason string }

func (e *RequirementError) Error() string {
	return fmt.Sprintf("Java需求（%s）：%s", e.Source, e.Reason)
}
func (e *RequirementError) DiagnosticCode() string { return "java-requirement" }
func requirementError(source, reason string) error { return &RequirementError{source, reason} }

type requirementKey struct{}

// WithRequirement carries only a cloned job value to the final process Probe.
// It never keeps UI state, services or an executable in a context.
func WithRequirement(ctx context.Context, r Requirement) (context.Context, error) {
	if err := r.Validate(); err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, requirementKey{}, r.Clone()), nil
}
func RequirementFromContext(ctx context.Context) (Requirement, bool) {
	r, ok := ctx.Value(requirementKey{}).(Requirement)
	return r.Clone(), ok
}
