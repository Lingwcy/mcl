package loader

import (
	"strings"
	"testing"
)

func TestLegacyToolClass8DoesNotRequireModernJVM(t *testing.T) {
	root, fixed := stagedProcessorJar(t, map[string][]byte{"test/Main.class": syntheticClass(52, 0), "module-info.class": syntheticClass(53, 0), "META-INF/versions/17/test/Main.class": syntheticClass(61, 0)})
	count := 0
	minimum, err := processorJarMinimum(t.Context(), root, "tool.jar", fixed, "test.Main", &count)
	if err != nil || minimum != 8 {
		t.Fatal("old tool incorrectly raised to a modern JVM", minimum, err)
	}
}
func TestManifestDependenciesCannotEscapeExplicitPins(t *testing.T) {
	main := "libraries/a/tool.jar"
	approved := []string{main, "libraries/b/dependency.jar"}
	manifest := []byte("Main-Class: test.Main" + string(byte(10)) + "Class-Path: ../b/dependency.jar" + string([]byte{10, 10}))
	if _, err := processorManifest(manifest, main, approved); err != nil {
		t.Fatal("fixed explicit dependency was rejected", err)
	}
	for _, reference := range []string{"https://example.invalid/code.jar", "file:/outside.jar", "../../outside.jar", "../b/unknown.jar"} {
		data := []byte(strings.ReplaceAll(string(manifest), "../b/dependency.jar", reference))
		if _, err := processorManifest(data, main, approved); err == nil {
			t.Fatal("implicit dependency escaped fixed classpath")
		}
	}
}
