package download

import (
	"strings"

	"mcl/internal/core/meta"
)

const aliyunMavenOrigin = "https://maven.aliyun.com/repository/central"

// Central's public endpoints share the same coordinate namespace. The selected
// bytes still need the original independent digest, size and task identity.
func centralAlternate(original string) string {
	location, err := meta.ParseMavenURL(original)
	if err != nil {
		return ""
	}
	switch location.Repository {
	case "central":
		location.Repository = "central-repo1"
	case "central-repo1":
		location.Repository = "central"
	default:
		return ""
	}
	if location.Coordinate.Extension != "jar" && location.Coordinate.Extension != "zip" {
		return ""
	}
	return location.URL()
}

// MavenMirrorURL returns a mainland mirror URL for fixed, coordinate-bound Maven
// .jar/.zip artifacts from known official repositories, or empty when none applies.
func MavenMirrorURL(original string) string {
	return mavenMirrorURL(original)
}

// mavenMirrorURL maps fixed, coordinate-bound .jar/.zip artifacts from known
// official Maven repositories to mainland mirrors. Callers still verify the
// full representation against an independent pinned digest and exact size;
// mutable metadata XML and .sha1 sidecars are always rejected by ParseMavenURL.
func mavenMirrorURL(original string) string {
	location, err := meta.ParseMavenURL(original)
	if err != nil {
		return ""
	}
	if location.Coordinate.Extension != "jar" && location.Coordinate.Extension != "zip" {
		return ""
	}
	if mutableMavenVersion(location.Coordinate.Version) {
		return ""
	}
	switch location.Repository {
	case "forge", "neoforge", "fabric", "minecraft":
		return mirrorOrigin + "/maven/" + location.Coordinate.Path()
	case "central", "central-repo1":
		return aliyunMavenOrigin + "/" + location.Coordinate.Path()
	default:
		return ""
	}
}

func mutableMavenVersion(version string) bool {
	if version == "" {
		return true
	}
	for _, token := range strings.FieldsFunc(strings.ToLower(version), func(r rune) bool { return r == '-' || r == '.' }) {
		switch token {
		case "latest", "release", "snapshot":
			return true
		}
	}
	return false
}
