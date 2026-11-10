package loader

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"slices"
	"strconv"
	"strings"

	"mcl/internal/core/meta"
)

// Forge publishes an explicit game->build mapping. NeoForge uses its Maven
// release coordinates; the installer independently verifies the exact game
// identity again before any processors are run.
func (c *Client) forgeReleases(ctx context.Context, kind, game string) ([]Release, error) {
	var versions []string
	recommended := ""
	if kind == "forge" {
		var err error
		versions, err = c.forgeVersions(ctx, game)
		if err != nil {
			return nil, err
		}
		data, err := c.bytes(ctx, c.ForgeURL+"/promotions_slim.json", 1<<20)
		if err == nil {
			var promotions struct {
				Promos map[string]string `json:"promos"`
			}
			if json.Unmarshal(data, &promotions) == nil {
				recommended = promotions.Promos[game+"-recommended"]
			}
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	} else {
		artifact := "neoforge"
		if game == "1.20.1" {
			artifact = "forge"
		}
		data, err := c.bytes(ctx, c.NeoForgeURL+"/"+artifact+"/maven-metadata.xml", 4<<20)
		if err != nil {
			return nil, err
		}
		var catalog struct {
			Versions []string `xml:"versioning>versions>version"`
		}
		if err := xml.Unmarshal(data, &catalog); err != nil {
			return nil, errors.New("NeoForge 版本目录无效")
		}
		for _, version := range catalog.Versions {
			if neoGame(version, game) {
				versions = append(versions, version)
			}
		}
	}
	if len(versions) > 10000 {
		return nil, errors.New("加载器版本数量过多")
	}
	seen := make(map[string]bool)
	result := make([]Release, 0, len(versions))
	for _, version := range versions {
		if kind == "forge" || game == "1.20.1" {
			version = strings.TrimPrefix(version, game+"-")
		}
		if !meta.ValidID(version) || strings.ContainsAny(version, " /\\:") || seen[version] {
			continue
		}
		seen[version] = true
		stable := !strings.Contains(version, "-")
		isRecommended := version == recommended && recommended != ""
		if kind == "forge" {
			stable = stableForgeBuild(version, game)
			// Promotions may omit the explicit game branch, but only decorate an
			// identity already returned by the official catalog; never invent it.
			build, branch, qualified := strings.Cut(version, "-")
			isRecommended = isRecommended || qualified && branch == game && build == recommended && recommended != ""
		}
		result = append(result, Release{Version: version, Stable: stable, Recommended: isRecommended})
	}
	slices.SortStableFunc(result, func(a, b Release) int { return -compareBuild(a.Version, b.Version) })
	return result, ctx.Err()
}

// A historical -<game> branch is not an EA label. Unknown qualifiers remain
// non-stable; do not remove them from the exact installer coordinate.
func stableForgeBuild(version, game string) bool {
	build, branch, qualified := strings.Cut(version, "-")
	if qualified && branch != game {
		return false
	}
	parts := strings.Split(build, ".")
	if len(parts) < 3 || len(parts) > 4 {
		return false
	}
	for _, part := range parts {
		if part == "" || strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) != -1 {
			return false
		}
	}
	return true
}

func neoGame(version, game string) bool {
	if game == "1.20.1" {
		return strings.HasPrefix(version, game+"-")
	}
	base, _, _ := strings.Cut(version, "-")
	v, g := strings.Split(base, "."), strings.Split(game, ".")
	if len(g) < 2 {
		return false
	}
	if g[0] == "1" {
		patch := "0"
		if len(g) == 3 {
			patch = g[2]
		} else if len(g) != 2 {
			return false
		}
		return len(v) == 3 && v[0] == g[1] && v[1] == patch
	}
	// The year-based game scheme retains all three game components; a
	// missing patch is zero, followed by NeoForge's own build component.
	if len(g) == 2 {
		g = append(g, "0")
	}
	return len(g) == 3 && len(v) == 4 && slices.Equal(v[:3], g)
}

func compareBuild(a, b string) int {
	aBase, aPre, aHas := strings.Cut(a, "-")
	bBase, bPre, bHas := strings.Cut(b, "-")
	x, y := strings.Split(aBase, "."), strings.Split(bBase, ".")
	for index := range max(len(x), len(y)) {
		av, bv := "0", "0"
		if index < len(x) {
			av = x[index]
		}
		if index < len(y) {
			bv = y[index]
		}
		an, ae := strconv.Atoi(av)
		bn, be := strconv.Atoi(bv)
		if ae == nil && be == nil {
			if an < bn {
				return -1
			}
			if an > bn {
				return 1
			}
		} else if order := strings.Compare(av, bv); order != 0 {
			return order
		}
	}
	if !aHas && bHas {
		return 1
	}
	if aHas && !bHas {
		return -1
	}
	x, y = strings.Split(aPre, "."), strings.Split(bPre, ".")
	for index := range min(len(x), len(y)) {
		an, ae := strconv.Atoi(x[index])
		bn, be := strconv.Atoi(y[index])
		if ae == nil && be == nil {
			if an < bn {
				return -1
			}
			if an > bn {
				return 1
			}
		} else if order := strings.Compare(x[index], y[index]); order != 0 {
			return order
		}
	}
	if len(x) < len(y) {
		return -1
	}
	if len(x) > len(y) {
		return 1
	}
	return 0
}

// The files-site JSON and Maven XML are both official indices. An unavailable
// primary index must not force guessing a suffix or selecting a newer build.
func (c *Client) forgeVersions(ctx context.Context, game string) ([]string, error) {
	ctx = WithResolutionBudget(ctx)
	data, primaryErr := c.bytes(ctx, c.ForgeURL+"/maven-metadata.json", 4<<20)
	if primaryErr == nil {
		var catalog map[string][]string
		if err := json.Unmarshal(data, &catalog); err == nil {
			versions := catalog[game]
			if len(versions) > 10000 {
				return nil, errors.New("Forge 版本数量过多")
			}
			return versions, ctx.Err()
		}
		primaryErr = errors.New("Forge 官方 JSON 目录无效")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	data, err := c.bytes(ctx, "https://maven.minecraftforge.net/net/minecraftforge/forge/maven-metadata.xml", 4<<20)
	if err != nil {
		return nil, errors.Join(primaryErr, err)
	}
	var catalog struct {
		Versions []string `xml:"versioning>versions>version"`
	}
	if err := xml.Unmarshal(data, &catalog); err != nil {
		return nil, errors.New("Forge 官方 Maven 目录无效")
	}
	if len(catalog.Versions) > 50000 {
		return nil, errors.New("Forge 官方 Maven 版本数量过多")
	}
	var versions []string
	for _, version := range catalog.Versions {
		if strings.HasPrefix(version, game+"-") {
			versions = append(versions, version)
		}
	}
	if len(versions) > 10000 {
		return nil, errors.New("Forge 游戏版本数量过多")
	}
	return versions, ctx.Err()
}
