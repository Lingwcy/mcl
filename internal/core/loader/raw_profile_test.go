package loader

import (
	"encoding/json"
	"strings"
	"testing"
)

func profileDeclaration(library map[string]any) []byte {
	data, _ := json.Marshal(map[string]any{"id": "fabric-fixture", "inheritsFrom": "1.20.4", "mainClass": "net.fabricmc.loader.impl.launch.knot.KnotClient", "libraries": []any{library}})
	return data
}
func TestLoaderMissingArtifactMayUseOnlyDeclaredRepositoryOrMojangDefault(t *testing.T) {
	for _, library := range []map[string]any{
		{"name": "org.example:library:1"},
		{"name": "org.example:library:1", "url": "https://maven.fabricmc.net/"},
		{"name": "org.example:library:1", "url": "https://maven.quiltmc.org/repository/release/", "downloads": map[string]any{}},
	} {
		if _, err := decodeLoaderProfile(profileDeclaration(library)); err != nil {
			t.Fatal(library, err)
		}
	}
}
func TestLoaderExplicitInvalidIdentityNeverFallsBackToSidecar(t *testing.T) {
	valid := map[string]any{"url": "https://maven.fabricmc.net/org/example/library/1/library-1.jar", "sha1": strings.Repeat("a", 40), "size": 100}
	for _, library := range []map[string]any{
		{"name": "org.example:library:1", "url": nil},
		{"name": "org.example:library:1", "url": ""},
		{"name": "org.example:library:1", "downloads": nil},
		{"name": "org.example:library:1", "downloads": map[string]any{"artifact": nil}},
		{"name": "org.example:library:1", "downloads": map[string]any{"artifact": map[string]any{}}},
		{"name": "org.example:library:1", "url": "https://maven.fabricmc.net/arbitrary/"},
	} {
		if _, err := decodeLoaderProfile(profileDeclaration(library)); err == nil {
			t.Fatal("invalid explicit declaration accepted", library)
		}
	}
	library := map[string]any{"name": "org.example:library:1", "downloads": map[string]any{"artifact": valid}}
	if _, err := decodeLoaderProfile(profileDeclaration(library)); err != nil {
		t.Fatal(err)
	}
	valid["path"] = nil
	if _, err := decodeLoaderProfile(profileDeclaration(library)); err == nil {
		t.Fatal("explicit null path accepted as missing")
	}
}
func TestLoaderUnknownJavaDialectIsNotSilentlyIgnored(t *testing.T) {
	var document map[string]any
	if err := json.Unmarshal(profileDeclaration(map[string]any{"name": "org.example:library:1"}), &document); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]any{"javaVersion": nil, "compatibleJavaMajors": []int{8, 17}, "arguments": nil} {
		document[key] = value
		data, _ := json.Marshal(document)
		if _, err := decodeLoaderProfile(data); err == nil {
			t.Fatal("explicit null or unknown dialect ignored", key)
		}
		delete(document, key)
	}
}
