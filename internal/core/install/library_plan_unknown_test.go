package install

import (
	"mcl/internal/core/meta"
	"testing"
)

func TestNativeContainerNameAloneCannotRemoveUnknownJavaArtifact(t *testing.T) {
	version := fixture(t, "1.20.4")
	library := selectedFixtureLibrary(t, "org.lwjgl.lwjgl:lwjgl-platform:custom-nonempty")
	library.Natives = map[string]string{"windows": "natives-windows"}
	version.Libraries = []meta.Library{library}
	platform := meta.Platform{OS: "osx", Arch: "amd64", Features: map[string]bool{"custom-feature": true}}
	plan, err := Build(version, platform)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Libraries) != 1 || plan.Libraries[0].Role != LibraryClasspath || len(plan.Classpath) != 2 {
		t.Fatal("unknown Java content was discarded", plan.Libraries)
	}
	platform.Features["custom-feature"] = false
	if !plan.Platform.Features["custom-feature"] {
		t.Fatal("plan retained caller feature map")
	}
}
