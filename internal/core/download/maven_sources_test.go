package download

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"mcl/internal/core/meta"
)

func TestMavenSourceCandidatesKeepFixedCoordinateAndExplicitMirrorPolicy(t *testing.T) {
	central := "https://repo.maven.apache.org/maven2/org/example/library/1/library-1.jar"
	repo1 := "https://repo1.maven.org/maven2/org/example/library/1/library-1.jar"
	control, err := NewTransferControl(Preferences{Source: SourceOfficial})
	if err != nil {
		t.Fatal(err)
	}
	if got := control.candidates(central); !slices.Equal(got, []string{central, repo1}) {
		t.Fatal(got)
	}
	c, _ := meta.ParseCoordinate("net.minecraftforge:forge:1.7.10-10.13.4.1614-1.7.10:installer")
	official := "https://maven.minecraftforge.net/" + c.Path()
	mirror := mirrorOrigin + "/maven/" + c.Path()
	if got := control.candidates(official); !slices.Equal(got, []string{official}) {
		t.Fatal("default enabled Forge third-party mirror", got)
	}
	if err := control.SetPreferences(Preferences{Source: SourceMirrorFirst}); err != nil {
		t.Fatal(err)
	}
	if got := control.candidates(official); !slices.Equal(got, []string{mirror, official}) {
		t.Fatal(got)
	}
	for _, address := range []string{
		official + ".sha1", official + "?token=secret", strings.ReplaceAll(official, "10.13.4.1614", "latest"),
		"https://maven.minecraftforge.net/net/minecraftforge/forge/maven-metadata.xml",
	} {
		if mirrorURL(address) != "" {
			t.Fatal("mutable or non-coordinate Maven URL mapped to mirror", address)
		}
	}
	if centralAlternate(central+".sha1") != "" || centralAlternate("https://repo.maven.apache.org/maven2/org/example/library/maven-metadata.xml") != "" {
		t.Fatal("mutable sidecar/catalog acquired alternate identity")
	}
	for address, wantMirror := range map[string]string{
		"https://maven.minecraftforge.net/net/minecraftforge/binarypatcher/1/binarypatcher-1.jar":          mirrorOrigin + "/maven/net/minecraftforge/binarypatcher/1/binarypatcher-1.jar",
		"https://maven.neoforged.net/releases/net/neoforged/neoforge/21.1.1/neoforge-21.1.1-installer.jar": mirrorOrigin + "/maven/net/neoforged/neoforge/21.1.1/neoforge-21.1.1-installer.jar",
		"https://maven.fabricmc.net/net/fabricmc/fabric-loader/0.16.0/fabric-loader-0.16.0.jar":            mirrorOrigin + "/maven/net/fabricmc/fabric-loader/0.16.0/fabric-loader-0.16.0.jar",
		central: aliyunMavenOrigin + "/org/example/library/1/library-1.jar",
	} {
		if got := mirrorURL(address); got != wantMirror {
			t.Fatalf("mirrorURL(%s) = %q, want %q", address, got, wantMirror)
		}
	}
	if got := control.candidates(central); !slices.Equal(got, []string{aliyunMavenOrigin + "/org/example/library/1/library-1.jar", central, repo1}) {
		t.Fatalf("central MirrorFirst candidates = %v", got)
	}
	if err := control.SetPreferences(Preferences{Source: SourceOfficial}); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{
		"https://maven.neoforged.net/releases/net/neoforged/neoforge/21.1.1/neoforge-21.1.1-installer.jar",
		"https://maven.fabricmc.net/net/fabricmc/fabric-loader/0.16.0/fabric-loader-0.16.0.jar",
	} {
		if got := control.candidates(address); !slices.Equal(got, []string{address}) {
			t.Fatalf("SourceOfficial enabled mirror for %s: %v", address, got)
		}
	}
}
func TestForgeMirrorCorruptionCannotChangePublishedIdentity(t *testing.T) {
	c, _ := meta.ParseCoordinate("net.minecraftforge:forge:1.12.2-14.23.5.2860:installer")
	original := "https://maven.minecraftforge.net/" + c.Path()
	client := NewClient(t.TempDir())
	control, err := NewTransferControl(Preferences{Source: SourceMirrorFirst})
	if err != nil {
		t.Fatal(err)
	}
	client.Transfers = control
	var hosts []string
	client.HTTP.Transport = policyRoundTripper(func(r *http.Request) (*http.Response, error) {
		hosts = append(hosts, r.URL.Host)
		body := "pinned"
		if r.URL.Host == "bmclapi2.bangbang93.com" {
			body = "tamper"
		}
		return policyResponse(r, body), nil
	})
	file := testFile(original, "cache/forge-installer.jar", "pinned")
	file.AllowedHosts = []string{"maven.minecraftforge.net"}
	if err := client.Run(t.Context(), []File{file}, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(client.Root, file.Path))
	if err != nil || string(data) != "pinned" {
		t.Fatal(string(data), err)
	}
	if !slices.Equal(hosts, []string{"bmclapi2.bangbang93.com", "maven.minecraftforge.net"}) {
		t.Fatal(hosts)
	}
}
func TestCentralOfficialAlternateStillVerifiesFullArtifact(t *testing.T) {
	original := "https://repo1.maven.org/maven2/org/example/library/1/library-1.jar"
	client := NewClient(t.TempDir())
	var hosts []string
	client.HTTP.Transport = policyRoundTripper(func(r *http.Request) (*http.Response, error) {
		hosts = append(hosts, r.URL.Host)
		response := policyResponse(r, "pinned")
		if r.URL.Host == "repo1.maven.org" {
			response.StatusCode = 404
		}
		return response, nil
	})
	file := testFile(original, "cache/library.jar", "pinned")
	if err := client.Run(t.Context(), []File{file}, nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(hosts, []string{"repo1.maven.org", "repo.maven.apache.org"}) {
		t.Fatal(hosts)
	}
}

func TestDownloadDropsMissingMirrorWithoutConsumingOfficialRetries(t *testing.T) {
	c, _ := meta.ParseCoordinate("net.fabricmc:tiny-remapper:0.10.1")
	original := "https://maven.fabricmc.net/" + c.Path()
	client := NewClient(t.TempDir())
	control, err := NewTransferControl(Preferences{Source: SourceMirrorFirst})
	if err != nil {
		t.Fatal(err)
	}
	client.Transfers = control
	var hosts []string
	officialCalls := 0
	client.HTTP.Transport = policyRoundTripper(func(r *http.Request) (*http.Response, error) {
		hosts = append(hosts, r.URL.Host)
		resp := policyResponse(r, "pinned")
		if r.URL.Host == "bmclapi2.bangbang93.com" {
			resp.StatusCode = http.StatusNotFound
			return resp, nil
		}
		officialCalls++
		if officialCalls < 3 {
			resp.StatusCode = http.StatusServiceUnavailable
			return resp, nil
		}
		return resp, nil
	})
	file := testFile(original, "cache/tiny-remapper.jar", "pinned")
	if err := client.Run(t.Context(), []File{file}, nil); err != nil {
		t.Fatal(err)
	}
	want := []string{"bmclapi2.bangbang93.com", "maven.fabricmc.net", "maven.fabricmc.net", "maven.fabricmc.net"}
	if !slices.Equal(hosts, want) {
		t.Fatalf("hosts = %v, want %v", hosts, want)
	}
}

func TestFailoverTimeoutBoundsTricklingPrimaryOnlyWithAlternateAndNoRateLimit(t *testing.T) {
	base := 2 * time.Minute
	if got := failoverTimeout(base, 4<<20, true, false); got < 30*time.Second || got >= 2*time.Minute {
		t.Fatalf("expected bounded failover budget <2m, got %v", got)
	}
	if got := failoverTimeout(base, 4<<20, false, false); got != base {
		t.Fatalf("single source budget changed: got %v, want %v", got, base)
	}
	if got := failoverTimeout(base, 4<<20, true, true); got != base {
		t.Fatalf("speed-limited budget changed: got %v, want %v", got, base)
	}
}
