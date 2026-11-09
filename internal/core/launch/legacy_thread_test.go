package launch

import (
	"path/filepath"
	"slices"
	"testing"

	"mcl/internal/core/meta"
)

func TestLegacyMacJava8TransportDoesNotRestoreFirstThread(t *testing.T) {
	o := legacyOptions(t, "--username ${auth_player_name} --gameDir ${game_directory} --uuid ${auth_uuid} --accessToken ${auth_access_token}")
	o.Platform = meta.Platform{OS: "osx", Arch: "amd64"}
	o.Java.Arch = "amd64"
	plan, err := Build(o, session(t))
	if err != nil {
		t.Fatal(err)
	}
	args := plan.ProcessArguments(filepath.Join(o.Root, "runtime", "classpath.jar"))
	if len(args) == 0 || slices.Contains(args, "-XstartOnFirstThread") || !slices.Contains(args, o.Version.MainClass) || !slices.Contains(args, "-cp") {
		t.Fatal("legacy Java8 transport changed launch threading/entry", plan.Preview())
	}
	if ext, _, err := plan.SupportFile(); err != nil || ext != ".jar" {
		t.Fatal("Java8 manifest-only classpath transport changed", ext, err)
	}
}
