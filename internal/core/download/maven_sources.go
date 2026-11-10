package download

import (
	"strings"

	"mcl/internal/core/meta"
)

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

// This intentionally covers only Forge's fixed installer/client/universal
// distribution. It does not map an arbitrary repository, Maven metadata,
// Fabric/Quilt catalogs, NeoForge, sidecars, processors or a mutable build URL.
func forgeDistributionMirror(original string) string {
	location, err := meta.ParseMavenURL(original)
	if err != nil || location.Repository != "forge" {
		return ""
	}
	c := location.Coordinate
	if c.Group != "net.minecraftforge" || c.Artifact != "forge" || c.Extension != "jar" {
		return ""
	}
	switch c.Classifier {
	case "installer", "client", "universal":
	default:
		return ""
	}
	// Fixed numeric game/build/branch components only; no latest, dynamic Maven
	// versions, snapshots or qualifiers are silently routed to a mirror.
	if c.Version == "" {
		return ""
	}
	for _, r := range c.Version {
		if r < '0' || r > '9' {
			if r != '.' && r != '-' {
				return ""
			}
		}
	}
	segments := strings.Split(c.Version, "-")
	if len(segments) < 2 || len(segments) > 3 || len(segments) == 3 && segments[0] != segments[2] {
		return ""
	}
	for _, segment := range segments {
		parts := strings.Split(segment, ".")
		if len(parts) < 2 || len(parts) > 4 {
			return ""
		}
		for _, part := range parts {
			if part == "" {
				return ""
			}
		}
	}
	return mirrorOrigin + "/maven/" + c.Path()
}
