package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

const maxMappedAssetSize int64 = 512 << 20
const maxMappedAssetsTotal int64 = 16 << 30

type AssetMapping struct {
	Name string
	File download.File
}
type AssetPlan struct {
	Files                   []download.File
	Mappings                []AssetMapping
	Virtual, MapToResources bool
}

func ParseAssetPlan(data []byte) (AssetPlan, error) {
	if len(data) > MaxAssetIndexSize {
		return AssetPlan{}, errors.New("asset index exceeds 16 MiB")
	}
	var index AssetIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return AssetPlan{}, fmt.Errorf("parse asset index: %w", err)
	}
	if index.Objects == nil || len(index.Objects) > 100_000 {
		return AssetPlan{}, errors.New("asset index missing objects or exceeds 100000 entries")
	}
	result := AssetPlan{Virtual: index.Virtual, MapToResources: index.MapToResources}
	byHash := make(map[string]download.File)
	names := make(map[string]string)
	nodes := make(map[string]bool)
	var total int64
	for name, object := range index.Objects {
		if !meta.ValidPath(name) || !archive.SafePath(name) || !meta.ValidSHA1(object.Hash) {
			return AssetPlan{}, errors.New("unsafe asset name or hash")
		}
		key := archive.Key(name)
		if old, ok := names[key]; ok && old != name {
			return AssetPlan{}, errors.New("asset names collide by case or Unicode normalization")
		}
		names[key] = name
		if directory, ok := nodes[key]; ok && directory {
			return AssetPlan{}, errors.New("asset file/directory collision")
		}
		nodes[key] = false
		for rawParent := path.Dir(name); rawParent != "."; rawParent = path.Dir(rawParent) {
			parent := archive.Key(rawParent)
			if old, ok := names[parent]; ok && old != rawParent {
				return AssetPlan{}, errors.New("asset parent aliases by case or normalization")
			}
			names[parent] = rawParent
			if directory, ok := nodes[parent]; ok && !directory {
				return AssetPlan{}, errors.New("asset parent is another file")
			}
			nodes[parent] = true
		}
		hash := strings.ToLower(object.Hash)
		if previous, ok := byHash[hash]; ok && previous.Size != object.Size {
			return AssetPlan{}, errors.New("conflicting asset size for one hash")
		}
		relative := hash[:2] + "/" + hash
		file := download.File{Path: "cache/assets/objects/" + relative, Download: meta.Download{URL: AssetObjectURL + relative, SHA1: hash, Size: object.Size}}
		if err := file.Validate(); err != nil {
			return AssetPlan{}, err
		}
		total += object.Size
		if object.Size > maxMappedAssetSize || total > maxMappedAssetsTotal {
			return AssetPlan{}, errors.New("asset mappings exceed 512MiB per item or 16GiB total")
		}
		byHash[hash] = file
		result.Mappings = append(result.Mappings, AssetMapping{Name: name, File: file})
	}
	for _, file := range byHash {
		result.Files = append(result.Files, file)
	}
	slices.SortFunc(result.Files, func(a, b download.File) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(result.Mappings, func(a, b AssetMapping) int { return strings.Compare(a.Name, b.Name) })
	return result, nil
}

func validateAssetMappings(mappings []AssetMapping) error {
	if len(mappings) > 100_000 {
		return errors.New("asset mapping count exceeds limit")
	}
	var total int64
	names := make(map[string]bool)
	aliases := make(map[string]string)
	nodes := make(map[string]bool)
	for _, mapping := range mappings {
		if !archive.SafePath(mapping.Name) || !meta.ValidPath(mapping.Name) {
			return errors.New("unsafe asset mapping")
		}
		if err := mapping.File.Validate(); err != nil {
			return err
		}
		hash := strings.ToLower(mapping.File.SHA1)
		if mapping.File.Path != "cache/assets/objects/"+hash[:2]+"/"+hash {
			return errors.New("asset mapping source is not a pinned shared object")
		}
		total += mapping.File.Size
		if mapping.File.Size > maxMappedAssetSize || total > maxMappedAssetsTotal {
			return errors.New("asset mapping byte limit exceeded")
		}
		key := archive.Key(mapping.Name)
		if names[key] || nodes[key] {
			return errors.New("asset mapping path collision")
		}
		names[key] = true
		for raw := mapping.Name; raw != "."; raw = path.Dir(raw) {
			key := archive.Key(raw)
			if old, ok := aliases[key]; ok && old != raw {
				return errors.New("asset mapping parent alias")
			}
			aliases[key] = raw
		}
		for parent := path.Dir(key); parent != "."; parent = path.Dir(parent) {
			if names[parent] {
				return errors.New("asset mapping file/parent collision")
			}
			nodes[parent] = true
		}
	}
	return nil
}
