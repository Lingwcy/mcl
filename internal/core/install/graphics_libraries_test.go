package install

import "testing"

func graphicsLibrary(coordinate string, role LibraryRole) LibraryFile {
	return LibraryFile{Coordinate: coordinate, Role: role}
}
func TestGraphicsChecksUseFinalSelectedIdentities(t *testing.T) {
	legacy := graphicsLibrary("org.lwjgl.lwjgl:lwjgl:2.9.2-nightly-20140822", LibraryClasspath)
	modern := graphicsLibrary("org.lwjgl:lwjgl:3.3.3", LibraryClasspath)
	native2 := graphicsLibrary("org.lwjgl.lwjgl:lwjgl-platform:2.9.2-nightly-20140822:natives-osx", LibraryNativeArchive)
	native3 := graphicsLibrary("org.lwjgl:lwjgl:3.3.3:natives-macos", LibraryNativeClasspath)
	for _, test := range []struct {
		name   string
		files  []LibraryFile
		family string
	}{
		{"legacy", []LibraryFile{legacy, native2}, "lwjgl2"},
		{"modern", []LibraryFile{modern, native3}, "lwjgl3"},
		{"mixed-api", []LibraryFile{legacy, modern}, ""},
		{"mixed-native", []LibraryFile{modern, native2}, ""},
		{"mixed-native-reversed", []LibraryFile{legacy, native3}, ""},
		{"partial-native-not-a-game", []LibraryFile{native2}, "unobserved"},
		{"legacy-wrong-version", []LibraryFile{legacy, graphicsLibrary("org.lwjgl.lwjgl:lwjgl-platform:2.9.4-nightly-20150209:natives-osx", LibraryNativeArchive)}, ""},
		{"modern-wrong-version", []LibraryFile{modern, graphicsLibrary("org.lwjgl:lwjgl:3.3.2:natives-macos", LibraryNativeClasspath)}, ""},
		{"active-asm-conflict", []LibraryFile{graphicsLibrary("org.ow2.asm:asm:9.5", LibraryClasspath), graphicsLibrary("org.ow2.asm:asm:9.6", LibraryClasspath)}, ""},
		{"mac-patch", []LibraryFile{modern, graphicsLibrary("org.lwjgl:lwjgl-freetype:3.3.3", LibraryClasspath), graphicsLibrary("org.lwjgl:lwjgl-freetype:3.3.3:natives-macos", LibraryNativeClasspath), graphicsLibrary("org.lwjgl:lwjgl-freetype:3.3.3:natives-macos-patch", LibraryNativeClasspath)}, "lwjgl3"},
	} {
		t.Run(test.name, func(t *testing.T) {
			report, err := checkGraphicsLibraries(test.files)
			if test.family == "" {
				if err == nil {
					t.Fatal("definite mismatch accepted", report)
				}
			} else if err != nil || report.Family != test.family {
				t.Fatal(report, err)
			}
		})
	}
}
