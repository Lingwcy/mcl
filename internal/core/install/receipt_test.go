package install

import (
	"strings"
	"testing"
	"time"

	"mcl/internal/core/instance"
	"mcl/internal/core/meta"
)

func TestInstalledLoaderReceiptsMatchExplicitForgeIdentity(t *testing.T) {
	for _, kind := range []string{"fabric", "quilt", "forge", "neoforge"} {
		id := instance.NewID()
		required := instance.Loader{Type: kind, Version: "14.23.5.2860"}
		receipt := Receipt{InstanceID: id, VersionID: "1.12.2", VersionSHA1: strings.Repeat("a", 40), OS: "windows", Arch: "amd64", NativeDirectory: "instances/" + id + "/natives/ready-" + strings.Repeat("A", 26), CompletedAt: time.Now().UTC(), Loader: &required, LoaderFingerprint: strings.Repeat("b", 64)}
		if err := receipt.Validate(); err != nil {
			t.Fatalf("supported loader receipt %s: %v", kind, err)
		}
		value := instance.Instance{ID: id, Version: meta.ManifestEntry{ID: receipt.VersionID, SHA1: receipt.VersionSHA1}, Loader: &required}
		if !receipt.Matches(value, meta.Platform{OS: "windows", Arch: "amd64"}) {
			t.Fatal("matching loader receipt rejected")
		}
		receipt.Loader = &instance.Loader{Type: "unknown", Version: required.Version}
		if err := receipt.Validate(); err == nil {
			t.Fatal("unknown loader receipt accepted")
		}
	}
}
