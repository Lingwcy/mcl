package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"mcl/internal/core/archive"
)

type readBudget struct {
	jars  int
	bytes int64
}

// Read inspects bounded declarations and declared nested JARs only. It never
// interprets bytecode, follows remote references, or writes archive contents.
func Read(ctx context.Context, reader io.ReaderAt, size int64) (Archive, error) {
	result := Archive{}
	z, err := archive.Open(reader, size)
	if err != nil {
		return result, err
	}
	budget := &readBudget{}
	for _, format := range []struct{ name, loader string }{{"fabric.mod.json", "fabric"}, {"quilt.mod.json", "quilt"}, {"META-INF/neoforge.mods.toml", "neoforge"}, {"META-INF/mods.toml", "forge"}} {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !z.Has(format.name) {
			continue
		}
		variant, err := readVariant(ctx, z, format.loader, format.name, "", 0, budget)
		if err != nil {
			result.Problems = append(result.Problems, format.name+": "+err.Error())
			continue
		}
		result.Variants = append(result.Variants, variant.Variant)
	}
	if len(result.Variants) == 0 && len(result.Problems) == 0 {
		result.Problems = append(result.Problems, "未找到受支持的模组声明（旧版和自定义加载器尚未接入）")
	}
	result.ReadBytes = budget.bytes
	return result, ctx.Err()
}

func readMember(ctx context.Context, z *archive.Zip, name string, limit int64, budget *readBudget) ([]byte, error) {
	data, err := z.Read(ctx, name, min(limit, (64<<20)-budget.bytes))
	if err == nil {
		budget.bytes += int64(len(data))
	}
	return data, err
}

func readVariant(ctx context.Context, z *archive.Zip, loader, name, location string, depth int, budget *readBudget) (parsedVariant, error) {
	data, err := readMember(ctx, z, name, 512<<10, budget)
	if err != nil {
		return parsedVariant{}, err
	}
	var result parsedVariant
	switch name {
	case "fabric.mod.json":
		result, err = fabric(data)
	case "quilt.mod.json":
		result, err = quilt(data)
	default:
		if name == "META-INF/mods.toml" {
			loader = "forge"
		}
		jarVersion := "${file.jarVersion}"
		if z.Has("META-INF/MANIFEST.MF") {
			raw, e := readMember(ctx, z, "META-INF/MANIFEST.MF", 64<<10, budget)
			if e != nil {
				return result, e
			}
			jarVersion = manifestVersion(raw)
		}
		result, err = forge(data, loader, jarVersion)
		if z.Has("META-INF/jarjar/metadata.json") {
			raw, e := readMember(ctx, z, "META-INF/jarjar/metadata.json", 512<<10, budget)
			if e != nil {
				return result, e
			}
			var metadata struct{ Jars []struct{ Path string } }
			if e := json.Unmarshal(raw, &metadata); e != nil {
				return result, errors.New("无效 JarJar 清单")
			}
			for _, jar := range metadata.Jars {
				result.jars = append(result.jars, jar.Path)
			}
		}
	}
	if err != nil {
		return result, err
	}
	for index := range result.Mods {
		result.Mods[index].Location, result.Mods[index].Nested = location, depth > 0
	}
	slices.Sort(result.jars)
	result.jars = slices.Compact(result.jars)
	if len(result.jars) > 128 {
		return result, errors.New("内嵌包数量过多")
	}
	for _, entry := range result.jars {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		problem := func(reason string) {
			result.Incomplete = true
			if len(result.Problems) < 8 {
				result.Problems = append(result.Problems, entry+": "+reason)
			}
		}
		if depth >= 4 || budget.jars >= 128 || budget.bytes >= 64<<20 {
			problem("内嵌包检查达到边界")
			break
		}
		if !archive.SafePath(entry) || !strings.HasSuffix(strings.ToLower(entry), ".jar") {
			problem("内嵌包路径无效")
			continue
		}
		remaining := min(int64(32<<20), (64<<20)-budget.bytes)
		body, err := readMember(ctx, z, entry, remaining, budget)
		if err != nil {
			problem(err.Error())
			continue
		}
		budget.jars++
		child, err := archive.Open(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			problem(err.Error())
			continue
		}
		childName := ""
		for _, candidate := range candidateNames(result.Loader) {
			if child.Has(candidate) {
				childName = candidate
				break
			}
		}
		if childName == "" {
			continue
		} // Plain bundled libraries need not declare a mod ID.
		nested, err := readVariant(ctx, child, result.Loader, childName, location+"!/"+entry, depth+1, budget)
		if err != nil {
			problem(err.Error())
			continue
		}
		if nested.Loader != result.Loader && !(result.Loader == "quilt" && nested.Loader == "fabric") {
			problem("内嵌包加载器不匹配")
			continue
		}
		result.Mods = append(result.Mods, nested.Mods...)
		result.Incomplete = result.Incomplete || nested.Incomplete
		for _, warning := range nested.Problems {
			if len(result.Problems) < 8 {
				result.Problems = append(result.Problems, warning)
			}
		}
		if len(result.Mods) > 512 {
			return result, fmt.Errorf("单文件模组数量超过 %d", 512)
		}
	}
	return result, nil
}

func candidateNames(loader string) []string {
	switch loader {
	case "fabric":
		return []string{"fabric.mod.json"}
	case "quilt":
		return []string{"quilt.mod.json", "fabric.mod.json"}
	case "neoforge":
		return []string{"META-INF/neoforge.mods.toml", "META-INF/mods.toml"}
	case "forge":
		return []string{"META-INF/mods.toml"}
	}
	return nil
}
