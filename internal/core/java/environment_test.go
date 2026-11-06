package java

import (
	"reflect"
	"testing"
)

func TestEnvironmentRemovesOnlyInheritedJavaOverrides(t *testing.T) {
	got := Environment([]string{"PATH=/bin", "HOME=/home/user", "_JAVA_OPTIONS=-javaagent:a.jar", "JAVA_TOOL_OPTIONS=-Xmx1m", "jdk_java_options=@unknown", "CLASSPATH=foreign.jar", "DISPLAY=:0"})
	want := []string{"PATH=/bin", "HOME=/home/user", "DISPLAY=:0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment %v", got)
	}
}
