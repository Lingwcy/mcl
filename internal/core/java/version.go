package java

import (
	"cmp"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Version follows Java's release/update and build components. Optional vendor
// text never supplies version numbers. Numbers is zero-padded for comparison;
// an absent build is distinct from an observed build zero.
type Version struct {
	Numbers    [8]uint32
	Pre        string
	Build      uint32
	BuildKnown bool
}

var legacyVersion = regexp.MustCompile("^1[.]([1-9][0-9]*)[.]([0-9]+)(?:_([0-9]+))?(?:-b([0-9]+))?(.*)$")
var updateVersion = regexp.MustCompile("^([1-9][0-9]*)u([0-9]+)(?:-b([0-9]+))?(.*)$")
var modernVersion = regexp.MustCompile("^([1-9][0-9]*(?:[.][0-9]+)*)(.*)$")

func ParseVersion(text string) (Version, error) {
	var v Version
	if len(text) == 0 || len(text) > 128 {
		return v, errors.New("Java版本为空或过长")
	}
	for _, c := range text {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._+-", c)) {
			return v, errors.New("Java版本包含无效字符")
		}
	}
	number := func(s string) (uint32, error) { n, e := strconv.ParseUint(s, 10, 32); return uint32(n), e }
	var rest string
	if match := legacyVersion.FindStringSubmatch(text); match != nil {
		var err error
		v.Numbers[0], err = number(match[1])
		if err != nil || v.Numbers[0] > 8 {
			return Version{}, errors.New("Java旧式版本无效")
		}
		v.Numbers[1], err = number(match[2])
		if err != nil {
			return Version{}, err
		}
		if match[3] != "" {
			v.Numbers[2], err = number(match[3])
			if err != nil {
				return Version{}, err
			}
		}
		if match[4] != "" {
			v.Build, err = number(match[4])
			if err != nil {
				return Version{}, err
			}
			v.BuildKnown = true
		}
		rest = match[5]
	} else if match := updateVersion.FindStringSubmatch(text); match != nil {
		var err error
		v.Numbers[0], err = number(match[1])
		if err != nil || v.Numbers[0] > 8 {
			return Version{}, errors.New("Java update格式无效")
		}
		v.Numbers[2], err = number(match[2])
		if err != nil {
			return Version{}, err
		}
		if match[3] != "" {
			v.Build, err = number(match[3])
			if err != nil {
				return Version{}, err
			}
			v.BuildKnown = true
		}
		rest = match[4]
	} else if match := modernVersion.FindStringSubmatch(text); match != nil {
		parts := strings.Split(match[1], ".")
		if len(parts) > len(v.Numbers) {
			return v, errors.New("Java版本分段过多")
		}
		for i, part := range parts {
			n, err := number(part)
			if err != nil {
				return Version{}, err
			}
			v.Numbers[i] = n
		}
		rest = match[2]
	} else {
		return v, errors.New("无法识别Java版本")
	}
	if v.Numbers[0] == 0 {
		return Version{}, errors.New("Java主版本无效")
	}
	// Java PRE is alphanumeric. Optional build information is opaque: e.g.
	// +7-LTS or +7-vendor.999 does not become another release component.
	preBuild, optional, hasBuild := strings.Cut(rest, "+")
	if preBuild != "" {
		if !strings.HasPrefix(preBuild, "-") {
			return Version{}, errors.New("Java版本后缀无效")
		}
		pre := strings.TrimPrefix(preBuild, "-")
		if strings.EqualFold(pre, "lts") {
			pre = ""
		}
		for _, c := range pre {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
				return Version{}, errors.New("Java预发布标识无效")
			}
		}
		if pre == "" && !strings.EqualFold(preBuild, "-lts") {
			return Version{}, errors.New("Java预发布标识为空")
		}
		v.Pre = pre
	}
	if hasBuild {
		if v.BuildKnown {
			return Version{}, errors.New("Java版本包含重复build")
		}
		build, info, hasInfo := strings.Cut(optional, "-")
		if build != "" {
			n, err := number(build)
			if err != nil {
				return Version{}, errors.New("Java build无效")
			}
			v.Build, v.BuildKnown = n, true
		} else if !hasInfo {
			return Version{}, errors.New("Java build为空")
		}
		if hasInfo && (info == "" || strings.Contains(info, "+")) {
			return Version{}, errors.New("Java附加标识无效")
		}
	}
	return v, nil
}
func (v Version) Major() int   { return int(v.Numbers[0]) }
func (v Version) Stable() bool { return v.Pre == "" }
func (v Version) CompareRelease(other Version) int {
	return slices.Compare(v.Numbers[:], other.Numbers[:])
}
func (v Version) Compare(other Version) int {
	if n := v.CompareRelease(other); n != 0 {
		return n
	}
	if v.Stable() != other.Stable() {
		if v.Stable() {
			return 1
		}
		return -1
	}
	if n := strings.Compare(v.Pre, other.Pre); n != 0 {
		return n
	}
	if v.BuildKnown != other.BuildKnown {
		if v.BuildKnown {
			return 1
		}
		return -1
	}
	return cmp.Compare(v.Build, other.Build)
}

// NormalizedVersion retains the public java.version. A separately observed
// java.runtime.version may supply build evidence only for the SAME release.
func (r Runtime) NormalizedVersion() (Version, error) {
	v, err := ParseVersion(r.Version)
	if err != nil {
		return Version{}, err
	}
	if v.Major() != r.Major {
		return Version{}, errors.New("Java版本与探测主版本不一致")
	}
	if r.RuntimeVersion != "" {
		detailed, err := ParseVersion(r.RuntimeVersion)
		if err != nil {
			return Version{}, errors.New("Java运行时版本无法识别")
		}
		if v.CompareRelease(detailed) != 0 || v.Stable() != detailed.Stable() {
			return Version{}, errors.New("Java运行时版本与公开版本不一致")
		}
		if v.BuildKnown && detailed.BuildKnown && v.Build != detailed.Build {
			return Version{}, errors.New("Java build证据不一致")
		}
		if detailed.BuildKnown {
			v.Build, v.BuildKnown = detailed.Build, true
		}
	}
	return v, nil
}
func CompareRuntimeVersions(a, b Runtime) int {
	av, ae := a.NormalizedVersion()
	bv, be := b.NormalizedVersion()
	if ae != nil || be != nil {
		if ae == nil {
			return 1
		}
		if be == nil {
			return -1
		}
		return 0
	}
	return av.Compare(bv)
}
