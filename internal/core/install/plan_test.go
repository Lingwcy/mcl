package install

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func fixture(t *testing.T, id string) meta.Version {
	t.Helper()
	data, err := os.ReadFile("../meta/testdata/" + id + ".json")
	if err != nil {
		t.Fatal(err)
	}
	v, err := meta.ParseVersion(data)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestOfficialPlansSelectNativeArchitecture(t *testing.T) {
	for _, id := range []string{"1.20.4", "1.21.1"} {
		for _, target := range []struct{ os, arch, native string }{{"osx", "arm64", "natives-macos-arm64"}, {"osx", "amd64", "natives-macos"}, {"windows", "amd64", "natives-windows"}, {"windows", "386", "natives-windows-x86"}, {"windows", "arm64", "natives-windows-arm64"}, {"linux", "amd64", "natives-linux"}} {
			t.Run(id+"/"+target.os+"/"+target.arch, func(t *testing.T) {
				v := fixture(t, id)
				p, err := Build(v, meta.Platform{OS: target.os, Arch: target.arch, Version: "10.0"})
				if err != nil {
					t.Fatal(err)
				}
				count := 0
				for _, file := range p.Files {
					if strings.Contains(file.Path, "-natives-") {
						count++
						if !strings.HasSuffix(file.Path, "-"+target.native+".jar") && !(target.os == "osx" && target.arch == "amd64" && strings.HasSuffix(file.Path, "-natives-macos-patch.jar")) {
							t.Errorf("wrong native: %s", file.Path)
						}
					}
				}
				want := 7
				if id == "1.21.1" {
					want = 8
				}
				if count != want || len(p.Natives) != 0 {
					t.Fatalf("modern natives count=%d extracted=%d", count, len(p.Natives))
				}
				if p.Classpath[len(p.Classpath)-1] != p.ClientPath || p.LoggingPath == "" || p.AssetIndexPath == "" {
					t.Fatal("missing runtime input")
				}
			})
		}
	}
}

func TestLegacyClassifierAndInvalidPlan(t *testing.T) {
	v := fixture(t, "1.20.4")
	if _, err := Build(v, meta.Platform{OS: "linux", Arch: "arm64"}); err == nil {
		t.Fatal("version without native ARM64 artifacts accepted")
	}
	var lib meta.Library
	if err := json.Unmarshal([]byte(`{"name":"org.example:native:1","natives":{"windows":"natives-windows-${arch}"},"downloads":{"classifiers":{"natives-windows-64":{"url":"https://example.com/native.jar","size":1,"sha1":"0000000000000000000000000000000000000000"}}},"extract":{"exclude":["META-INF/"]}}`), &lib); err != nil {
		t.Fatal(err)
	}
	v.Libraries = []meta.Library{lib}
	p, err := Build(v, meta.Platform{OS: "windows", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Natives) != 1 || !strings.HasSuffix(p.Natives[0].File.Path, "natives-windows-64.jar") || len(p.Classpath) != 1 {
		t.Fatalf("legacy plan %+v", p)
	}
	if _, err := Build(v, meta.Platform{OS: "windows", Arch: "arm64"}); err == nil {
		t.Fatal("legacy x86 substitution allowed on ARM")
	}
	v = fixture(t, "1.21.1")
	v.Libraries[0].Downloads.Artifact.Path = "../escape.jar"
	if _, err := Build(v, meta.CurrentPlatform()); err == nil {
		t.Fatal("unsafe library path accepted")
	}
	if _, err := modernNativeMatches("natives-unknown", meta.CurrentPlatform()); err == nil {
		t.Fatal("unknown native format accepted")
	}
}

func TestAssetIndexPlanAndBoundaries(t *testing.T) {
	hash := "0123456789012345678901234567890123456789"
	index := AssetIndex{Objects: map[string]AssetObject{"a/sound.ogg": {Hash: hash, Size: 12}, "b/sound.ogg": {Hash: strings.ToUpper(hash), Size: 12}}}
	data, _ := json.Marshal(index)
	files, err := PlanAssets(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "cache/assets/objects/01/"+hash || files[0].URL != AssetObjectURL+"01/"+hash {
		t.Fatalf("objects %+v", files)
	}
	index.Objects["c"] = AssetObject{Hash: hash, Size: 13}
	data, _ = json.Marshal(index)
	if _, err := PlanAssets(data); err == nil {
		t.Fatal("conflicting hash size accepted")
	}
	for _, body := range []string{`{}`, `{"objects":{},"virtual":true}`, `{"objects":{},"map_to_resources":true}`, `{"objects":{"../a":{"hash":"` + hash + `","size":1}}}`, `{"objects":{"a":{"hash":"bad","size":1}}}`} {
		if _, err := PlanAssets([]byte(body)); err == nil {
			t.Fatalf("invalid asset index accepted: %s", body)
		}
	}
}
