package install

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

type rejectLegacyMetadataNetwork struct{}

func (rejectLegacyMetadataNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected network in official metadata regression")
}
func officialLegacy1122(t *testing.T) meta.Version {
	t.Helper()
	raw, err := os.ReadFile("../meta/testdata/1.12.2.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../meta/testdata/1.12.2-source.json")
	if err != nil {
		t.Fatal(err)
	}
	var entry meta.ManifestEntry
	if err := json.Unmarshal(source, &entry); err != nil {
		t.Fatal(err)
	}
	digest := sha1.Sum(raw)
	if entry.ID != "1.12.2" || entry.SHA1 != "832d95b9f40699d4961394dcf6cf549e65f15dc5" || hex.EncodeToString(digest[:]) != entry.SHA1 {
		t.Fatal("official fixture identity mismatch")
	}
	client := meta.NewClient(t.TempDir())
	client.HTTP = &http.Client{Transport: rejectLegacyMetadataNetwork{}}
	client.VerifiedFetch = func(ctx context.Context, address, digest string, limit int64) ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if address != entry.URL || digest != entry.SHA1 || int64(len(raw)) > limit {
			return nil, errors.New("unexpected fixed metadata request")
		}
		return slices.Clone(raw), nil
	}
	version, err := client.Version(t.Context(), entry)
	if err != nil {
		t.Fatal(err)
	}
	return version
}
func TestOfficial1122MacFallbackPreservesOptionalJavaLibrary(t *testing.T) {
	version := officialLegacy1122(t)
	host := meta.Platform{OS: "osx", Arch: "arm64", Version: "27.2"}
	if _, err := Build(version, host); !errors.Is(err, ErrNativeArchitecture) {
		t.Fatal("missing true ARM64 limitation", err)
	}
	target, err := SelectPlatform(version, host)
	if err != nil || target.Arch != "amd64" || host.Arch != "arm64" {
		t.Fatal(target, err)
	}
	plan, err := Build(version, target)
	if err != nil {
		t.Fatal(err)
	}
	speech := false
	lwjgl := false
	for _, file := range plan.Files {
		if strings.HasSuffix(file.Path, "/text2speech-1.10.3.jar") {
			speech = true
			if !slices.Contains(plan.Classpath, file.Path) {
				t.Fatal("optional native removed Java artifact")
			}
		}
		if strings.Contains(file.Path, "text2speech-1.10.3-natives-") {
			t.Fatal("non-macOS native selected", file.Path)
		}
	}
	for _, native := range plan.Natives {
		if strings.Contains(native.File.Path, "lwjgl-platform-2.9.2-nightly-20140822-natives-osx.jar") {
			lwjgl = true
		}
		if strings.Contains(native.File.Path, "natives-windows") || strings.Contains(native.File.Path, "natives-linux") {
			t.Fatal("mixed native OS", native.File.Path)
		}
	}
	if !speech || !lwjgl || len(plan.Natives) != 3 {
		t.Fatal(speech, lwjgl, len(plan.Natives))
	}
}
func TestOfficial1122WindowsLinuxStillSelectDeclaredSpeechNative(t *testing.T) {
	version := officialLegacy1122(t)
	for _, name := range []string{"windows", "linux"} {
		t.Run(name, func(t *testing.T) {
			plan, err := Build(version, meta.Platform{OS: name, Arch: "amd64", Version: "10.0"})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, native := range plan.Natives {
				if strings.HasSuffix(native.File.Path, "text2speech-1.10.3-natives-"+name+".jar") {
					found = true
				}
			}
			if !found {
				t.Fatal("declared native was dropped", name)
			}
		})
	}
}
func TestDeclaredMacNativeMissingStillRejectsFallbackAndExposesCause(t *testing.T) {
	version := officialLegacy1122(t)
	for index := range version.Libraries {
		lib := &version.Libraries[index]
		if lib.Name == "org.lwjgl.lwjgl:lwjgl-platform:2.9.2-nightly-20140822" {
			delete(lib.Downloads.Classifiers, "natives-osx")
		}
	}
	target, err := SelectPlatform(version, meta.Platform{OS: "osx", Arch: "arm64"})
	var failure *PlatformFallbackError
	if target.Arch != "arm64" || !errors.As(err, &failure) || !errors.Is(err, ErrNativeArchitecture) {
		t.Fatal(target, err)
	}
	if !strings.Contains(err.Error(), "missing classifier natives-osx") {
		t.Fatal("fallback failure was hidden", err)
	}
}
