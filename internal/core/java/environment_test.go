package java

import (
	"reflect"
	"slices"
	"testing"
)

func TestEnvironmentRemovesOnlyInheritedJavaOverrides(t *testing.T) {
	got := Environment([]string{"PATH=/bin", "HOME=/home/user", "_JAVA_OPTIONS=-javaagent:a.jar", "JAVA_TOOL_OPTIONS=-Xmx1m", "jdk_java_options=@unknown", "CLASSPATH=foreign.jar", "DISPLAY=:0"})
	want := []string{"PATH=/bin", "HOME=/home/user", "DISPLAY=:0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment %v", got)
	}
}

func TestJavaEnvironmentDoesNotExportLauncherSecrets(t *testing.T) {
	parent := []string{"PATH=/safe/bin", "LANG=en_US.UTF-8", "MCL_CURSEFORGE_API_KEY=private", "GRUN_CURSEFORGE_API_KEY=private", "MYGO_UPDATER_PRIVATE_KEY=private"}
	result := Environment(parent)
	if !slices.Equal(result, parent[:2]) {
		t.Fatal("launcher credentials were exported to Java")
	}
	if len(parent) != 5 {
		t.Fatal("mutated caller environment")
	}
}
