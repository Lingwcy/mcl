package install

import (
	"encoding/json"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func TestAssetMappingFlagsDedupAndPathCollisions(t *testing.T) {
	hash := strings.Repeat("a", 40)
	for _, flags := range []struct{ virtual, resources bool }{{true, false}, {false, true}, {true, true}} {
		index := AssetIndex{Objects: map[string]AssetObject{"sounds/a.ogg": {Hash: hash, Size: 3}, "sounds/b.ogg": {Hash: hash, Size: 3}}, Virtual: flags.virtual, MapToResources: flags.resources}
		data, _ := json.Marshal(index)
		plan, err := ParseAssetPlan(data)
		if err != nil || len(plan.Files) != 1 || len(plan.Mappings) != 2 || plan.Virtual != flags.virtual || plan.MapToResources != flags.resources {
			t.Fatalf("mapping plan %+v %v", plan, err)
		}
	}
	for _, names := range [][]string{{"a", "a/b"}, {"Sounds/a", "sounds/b"}, {"x/A", "x/a"}, {"音效/é", "音效/é"}, {"../escape"}, {"a/NUL"}, {"/absolute"}} {
		index := AssetIndex{Objects: map[string]AssetObject{}}
		for _, name := range names {
			index.Objects[name] = AssetObject{Hash: hash, Size: 3}
		}
		data, _ := json.Marshal(index)
		if _, err := ParseAssetPlan(data); err == nil {
			t.Fatalf("ambiguous mapping accepted: %v", names)
		}
	}
	index := AssetIndex{Objects: map[string]AssetObject{"large": {Hash: hash, Size: maxMappedAssetSize + 1}}}
	data, _ := json.Marshal(index)
	if _, err := ParseAssetPlan(data); err == nil {
		t.Fatal("mapping item limit ignored")
	}
}

func TestLegacyNativeClassifierOSAndBitness(t *testing.T) {
	for _, tc := range []struct {
		classifier, os, arch string
		want                 bool
	}{
		{"natives-windows-64", "windows", "amd64", true}, {"natives-windows-64", "windows", "386", false},
		{"natives-windows-32", "windows", "386", true}, {"natives-linux", "linux", "amd64", true},
		{"natives-linux", "linux", "386", true}, {"natives-linux", "linux", "arm64", false},
		{"natives-osx", "osx", "amd64", true}, {"natives-macos-arm64", "osx", "arm64", true},
		{"natives-linux-aarch64", "linux", "arm64", true}, {"natives-linux-aarch64", "windows", "arm64", false},
	} {
		got, err := legacyNativeMatches(tc.classifier, meta.Platform{OS: tc.os, Arch: tc.arch})
		if err != nil || got != tc.want {
			t.Fatalf("native classifier %+v: %v %v", tc, got, err)
		}
	}
	for _, bad := range []string{"", "../META-INF/", "/", "/absolute", "a//b"} {
		if err := validateNativeExclusions([]string{bad}); err == nil {
			t.Fatal("unsafe exclusion accepted")
		}
	}
	if err := validateNativeExclusions([]string{"META-INF/", "skip"}); err != nil {
		t.Fatal(err)
	}
}
