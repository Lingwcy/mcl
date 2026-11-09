package download

import (
	"strings"
	"testing"
)

func TestPinnedTemurinMirrorMappingHonorsSourceOrder(t *testing.T) {
	official := "https://github.com/adoptium/temurin8-binaries/releases/download/jdk8u512-b09/OpenJDK8U-jdk_x64_mac_hotspot_8u512b09.tar.gz"
	mirror := "https://mirrors.tuna.tsinghua.edu.cn/Adoptium/8/jdk/x64/mac/OpenJDK8U-jdk_x64_mac_hotspot_8u512b09.tar.gz"
	if got := mirrorURL(official); got != mirror {
		t.Fatal(got)
	}
	modern := "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.9+10/OpenJDK21U-jre_aarch64_mac_hotspot_21.0.9_10.tar.gz"
	if mirrorURL(modern) == "" {
		t.Fatal("modern archive underscore mapping missing")
	}
	if mirrorURL(strings.Replace(modern, "jdk-21.0.9+10", "jdk-21.0.9%2B10", 1)) == "" {
		t.Fatal("official escaped release tag could not use mirror")
	}
	for _, mode := range []SourceMode{SourceOfficial, SourceOfficialFirst, SourceMirrorFirst, SourceAuto} {
		control, _ := NewTransferControl(Preferences{Source: mode})
		sources := control.candidates(official)
		if mode == SourceOfficial {
			if len(sources) != 1 || sources[0] != official {
				t.Fatal(sources)
			}
			continue
		}
		if len(sources) != 2 {
			t.Fatal(sources)
		}
		if mode == SourceMirrorFirst && sources[0] != mirror || mode != SourceMirrorFirst && sources[0] != official {
			t.Fatal(sources)
		}
	}
	for _, bad := range []string{official + "?token=secret", "https://github.com/other/temurin8-binaries/releases/download/v/OpenJDK8U-jdk_x64_mac_hotspot_8u512b09.tar.gz", "https://cdn.azul.com/zulu/bin/zulu8-ca-jdk8.0.512-macosx_aarch64.tar.gz"} {
		if mirrorURL(bad) != "" {
			t.Fatal("unrelated or authenticated URL mirrored", bad)
		}
	}
}
