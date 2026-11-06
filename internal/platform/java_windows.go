package platform

import (
	"context"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const javaBinary = "java.exe"
const systemJavaShim = ""

func registeredJava(ctx context.Context) []string {
	var paths []string
	for _, hive := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			for _, key := range []string{
				`SOFTWARE\JavaSoft\Java Runtime Environment`, `SOFTWARE\JavaSoft\Java Development Kit`,
				`SOFTWARE\JavaSoft\JRE`, `SOFTWARE\JavaSoft\JDK`,
				`SOFTWARE\Eclipse Adoptium`, `SOFTWARE\AdoptOpenJDK`,
				`SOFTWARE\Microsoft\JDK`, `SOFTWARE\Azul Systems\Zulu`, `SOFTWARE\BellSoft\Liberica`,
			} {
				paths = append(paths, javaRegistryHomes(ctx, hive, key, view, 0)...)
			}
		}
	}
	for _, variable := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		base := os.Getenv(variable)
		if !filepath.IsAbs(base) {
			continue
		}
		for _, directory := range []string{"Java", "Eclipse Adoptium", "AdoptOpenJDK", "Microsoft", "BellSoft", "Zulu", "Amazon Corretto", "Programs/Eclipse Adoptium"} {
			homes, _ := filepath.Glob(filepath.Join(base, filepath.FromSlash(directory), "*"))
			for _, home := range homes {
				paths = append(paths, JavaExecutable(home))
			}
		}
	}
	return paths
}

func javaRegistryHomes(ctx context.Context, hive registry.Key, name string, view uint32, depth int) []string {
	if ctx.Err() != nil || depth > 4 {
		return nil
	}
	key, err := registry.OpenKey(hive, name, registry.READ|view)
	if err != nil {
		return nil
	}
	defer key.Close()
	var paths []string
	for _, field := range []string{"JavaHome", "InstallationPath", "Path"} {
		if home, _, err := key.GetStringValue(field); err == nil && filepath.IsAbs(home) {
			paths = append(paths, JavaExecutable(home))
		}
	}
	children, _ := key.ReadSubKeyNames(256)
	for _, child := range children {
		paths = append(paths, javaRegistryHomes(ctx, hive, name+`\`+child, view, depth+1)...)
	}
	return paths
}
