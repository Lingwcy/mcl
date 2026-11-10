package java

import (
	"errors"
	"strings"
)

// ReleaseVersion interprets the vendor catalog dialect. Adoptium semver may
// encode build information as +10.0.LTS; only its first component is the Java
// build. The remaining metadata never becomes a Java update/patch number.
func (p Package) ReleaseVersion() (Version, error) {
	text := p.Version
	if p.Vendor() == "temurin" {
		base, build, found := strings.Cut(text, "+")
		number, optional, dotted := strings.Cut(build, ".")
		if found && dotted && !strings.Contains(number, "-") {
			if number == "" || optional == "" || strings.ContainsAny(build, "+-") {
				return Version{}, errors.New("Temurin目录版本格式无效")
			}
			text = base + "+" + number + "-" + optional
		}
	}
	return ParseVersion(text)
}
func (p Package) CheckRuntime(r Runtime) error {
	requested, err := p.ReleaseVersion()
	if err != nil {
		return err
	}
	actual, err := r.NormalizedVersion()
	if err != nil {
		return err
	}
	if requested.Major() != p.Major || actual.Major() != p.Major || !requested.Stable() || !actual.Stable() || r.EarlyAccess || actual.CompareRelease(requested) != 0 {
		return errors.New("安装的Java版本与固定官方包不一致")
	}
	if requested.BuildKnown && (!actual.BuildKnown || requested.Build != actual.Build) {
		return errors.New("安装的Java缺少对应的固定build证据")
	}
	return nil
}
