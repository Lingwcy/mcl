package install

import (
	"encoding/json"
	"errors"
	"testing"

	"mcl/internal/core/meta"
)

func TestLegacyMacPlanFallsBackAsAWholeToAMD64(t *testing.T) {
	version := fixture(t, "1.20.4")
	var lib meta.Library
	if err := json.Unmarshal([]byte(`{"name":"org.lwjgl.lwjgl:lwjgl-platform:2.9.1","natives":{"osx":"natives-osx"},"downloads":{"classifiers":{"natives-osx":{"url":"https://libraries.minecraft.net/native.jar","size":1,"sha1":"0000000000000000000000000000000000000000"}}},"extract":{"exclude":["META-INF/"]}}`), &lib); err != nil {
		t.Fatal(err)
	}
	version.Libraries = []meta.Library{lib}
	host := meta.Platform{OS: "osx", Arch: "arm64", Version: "15.0", Features: map[string]bool{"test": true}}
	if _, err := Build(version, host); !errors.Is(err, ErrNativeArchitecture) {
		t.Fatal("native architecture evidence missing", err)
	}
	target, err := SelectPlatform(version, host)
	if err != nil || target.OS != "osx" || target.Arch != "amd64" || target.Version != host.Version || !target.Features["test"] || host.Arch != "arm64" {
		t.Fatal(target, err)
	}
	plan, err := Build(version, target)
	if err != nil || plan.Platform.Arch != "amd64" || len(plan.Natives) != 1 {
		t.Fatal(plan, err)
	}
	for _, other := range []meta.Platform{{OS: "linux", Arch: "arm64"}, {OS: "windows", Arch: "arm64"}} {
		if _, err := SelectPlatform(version, other); err == nil {
			t.Fatal("translation invented for non-Mac host")
		}
	}
	delete(version.Libraries[0].Downloads.Classifiers, "natives-osx")
	if target, err := SelectPlatform(version, host); err == nil || target.Arch != "arm64" {
		t.Fatal("missing fallback artifact accepted", target, err)
	}
}
func TestModernNativeMacPlanDoesNotFallBackOrMaskInvalidMetadata(t *testing.T) {
	version := fixture(t, "1.20.4")
	host := meta.Platform{OS: "osx", Arch: "arm64", Version: "15.0"}
	target, err := SelectPlatform(version, host)
	if err != nil || target.Arch != "arm64" {
		t.Fatal(target, err)
	}
	file := version.Downloads["client"]
	file.SHA1 = "invalid"
	version.Downloads["client"] = file
	target, err = SelectPlatform(version, host)
	if err == nil || target.Arch != "arm64" {
		t.Fatal("invalid pinned metadata became translated target", target, err)
	}
}
