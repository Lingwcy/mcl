package loader

import (
	"path"
	"strings"
	"testing"
)

func TestMavenChecksumFormatsBindRequestedArtifact(t *testing.T) {
	relative := "org/example/library/1.2.3/library-1.2.3.jar"
	digest := strings.Repeat("a", 40)
	for _, text := range []string{
		digest, strings.ToUpper(digest), digest + "  " + path.Base(relative), digest + " *" + relative,
		"SHA1 (" + path.Base(relative) + ") = " + digest,
		"SHA-1 (" + relative + ") = " + strings.ToUpper(digest),
	} {
		got, err := parseMavenSHA1([]byte(text), relative)
		if err != nil || got != digest {
			t.Fatal(text, got, err)
		}
	}
	for _, text := range []string{
		digest + " other.jar", digest + " ../../library-1.2.3.jar", digest + " extra ignored", digest + string(rune(10)) + strings.Repeat("b", 40),
		"SHA1 (other.jar) = " + digest, "SHA1 () = " + digest, strings.Repeat("a", 64), "checksum=" + digest,
	} {
		if _, err := parseMavenSHA1([]byte(text), relative); err == nil {
			t.Fatal("ambiguous or foreign checksum accepted", text)
		}
	}
}
func TestMavenMetadataCannotRedirectRelativeIdentity(t *testing.T) {
	relative := "org/example/library/1/library-1.jar"
	if err := validateMavenLocation("https://maven.minecraftforge.net/"+relative, relative); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{
		"https://maven.minecraftforge.net/other.jar",
		"https://maven.minecraftforge.net/" + relative + "?token=secret",
		"https://maven.minecraftforge.net/" + relative + "?",
		"https://user:secret@maven.minecraftforge.net/" + relative,
		"https://maven.minecraftforge.net/../" + relative,
		"https://untrusted.invalid/" + relative,
	} {
		if err := validateMavenLocation(address, relative); err == nil {
			t.Fatal("foreign Maven identity accepted", address)
		}
	}
}
