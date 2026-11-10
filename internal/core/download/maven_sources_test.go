package download

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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
		"https://maven.minecraftforge.net/net/minecraftforge/binarypatcher/1/binarypatcher-1.jar",
		"https://maven.neoforged.net/releases/net/neoforged/neoforge/21.1.1/neoforge-21.1.1-installer.jar",
		"https://maven.fabricmc.net/net/fabricmc/fabric-loader/0.16.0/fabric-loader-0.16.0.jar",
	} {
		if mirrorURL(address) != "" {
			t.Fatal("unproven Maven mirror mapping", address)
		}
	}
	if centralAlternate(central+".sha1") != "" || centralAlternate("https://repo.maven.apache.org/maven2/org/example/library/maven-metadata.xml") != "" {
		t.Fatal("mutable sidecar/catalog acquired alternate identity")
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
