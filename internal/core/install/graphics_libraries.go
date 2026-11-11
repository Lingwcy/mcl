package install

import (
	"errors"
	"fmt"
	"strings"

	"mcl/internal/core/meta"
)

// GraphicsLibraries describes declared, selected components, NOT observed ABI,
// linkage, OpenGL support or game-window readiness.
type GraphicsLibraries struct {
	Family        string
	Java, Natives []string
}

func checkGraphicsLibraries(libraries []LibraryFile) (GraphicsLibraries, error) {
	result := GraphicsLibraries{Family: "unobserved"}
	modules := make(map[string]string)
	modernJava := make(map[string]string)
	legacyJava, legacyUtil := "", ""
	native2, native3 := false, false
	for _, library := range libraries {
		c, err := meta.ParseCoordinate(library.Coordinate)
		if err != nil {
			return result, err
		}
		if library.Role != LibraryRuntimeLookup {
			module := c.Group + ":" + c.Artifact + ":" + c.Classifier + "@" + c.Extension
			if previous, ok := modules[module]; ok && previous != c.Version {
				return result, fmt.Errorf("同一运行模块存在多个已启用版本: %s", module)
			}
			modules[module] = c.Version
		}
		if c.Group == "org.lwjgl.lwjgl" {
			if library.Role == LibraryClasspath && c.Classifier == "" {
				switch c.Artifact {
				case "lwjgl":
					legacyJava = c.Version
					result.Java = append(result.Java, c.String())
				case "lwjgl_util":
					legacyUtil = c.Version
					result.Java = append(result.Java, c.String())
				}
			}
			if c.Artifact == "lwjgl-platform" && library.Role == LibraryNativeArchive {
				native2 = true
				result.Natives = append(result.Natives, c.String())
			}
		}
		if c.Group == "org.lwjgl" && strings.HasPrefix(c.Artifact, "lwjgl") {
			switch library.Role {
			case LibraryClasspath:
				if c.Classifier == "" {
					modernJava[c.Artifact] = c.Version
					result.Java = append(result.Java, c.String())
				}
			case LibraryNativeClasspath:
				native3 = true
				result.Natives = append(result.Natives, c.String())
			}
		}
	}
	core3 := modernJava["lwjgl"]
	if legacyJava != "" && (core3 != "" || native3) || core3 != "" && native2 {
		return result, errors.New("运行计划混用了LWJGL2与LWJGL3，未自动替换图形库")
	}
	if legacyJava != "" {
		result.Family = "lwjgl2"
		if legacyUtil != "" && legacyUtil != legacyJava {
			return result, errors.New("LWJGL2 Java组件与本地库版本不一致")
		}
	} else if core3 != "" {
		result.Family = "lwjgl3"
	}
	for _, library := range libraries {
		c, _ := meta.ParseCoordinate(library.Coordinate)
		if legacyJava != "" && library.Role == LibraryNativeArchive && c.Group == "org.lwjgl.lwjgl" && c.Artifact == "lwjgl-platform" && c.Version != legacyJava {
			return result, errors.New("LWJGL2 Java组件与本地库版本不一致")
		}
		if library.Role != LibraryNativeClasspath {
			continue
		}
		if c.Group != "org.lwjgl" {
			continue
		}
		if version, ok := modernJava[c.Artifact]; ok && version != c.Version {
			return result, fmt.Errorf("LWJGL3模块%s的Java与本地库版本不一致", c.Artifact)
		}
	}
	return result, nil
}
