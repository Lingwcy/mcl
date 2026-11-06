package java

import "strings"

// Environment removes inherited JVM option injection. The selected executable
// and launch plan determine options; parent-shell Agent/classpath settings must
// not silently change a vanilla launch or a diagnostic probe.
func Environment(parent []string) []string {
	result := make([]string, 0, len(parent))
	for _, entry := range parent {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "_JAVA_OPTIONS", "JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS", "CLASSPATH":
			continue
		}
		result = append(result, entry)
	}
	return result
}
