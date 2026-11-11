package launch

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func lookupOptions(t *testing.T) Options {
	t.Helper()
	o := options(t, "1.20.4")
	o.Version.LibraryDirectory = "cache/loader-runtimes/" + strings.Repeat("a", 64) + "/ready-" + strings.Repeat("A", 26) + "/libraries"
	coordinate, _ := meta.ParseCoordinate("net.minecraftforge:forge:1.20.4-49.0.3:client")
	o.Version.RuntimeLookups = []meta.Download{{Path: coordinate.Path(), SHA1: strings.Repeat("b", 40), Size: 128}}
	return o
}
func TestRuntimeLookupDirectoryIsResolvedOnceWithoutChangingClasspath(t *testing.T) {
	o := lookupOptions(t)
	vanilla := o
	vanilla.Version.RuntimeLookups = nil
	vanilla.Version.LibraryDirectory = ""
	original, err := PreviewOptions(vanilla)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := PreviewOptions(o)
	if err != nil {
		t.Fatal(err)
	}
	expected := "-DlibraryDirectory=" + filepath.Join(o.Root, filepath.FromSlash(o.Version.LibraryDirectory))
	count := 0
	for _, arg := range preview.Arguments {
		if strings.HasPrefix(arg, "-DlibraryDirectory=") {
			count++
			if arg != expected {
				t.Fatal(arg)
			}
		}
	}
	if count != 1 {
		t.Fatal("missing unique lookup property", preview.Arguments)
	}
	cp := func(args []string) string { return args[slices.Index(args, "-cp")+1] }
	if cp(preview.Arguments) != cp(original.Arguments) {
		t.Fatal("runtime lookup changed classpath")
	}
	args := *o.Version.Arguments
	args.JVM = append(slices.Clone(args.JVM), meta.Argument{Values: []string{"-DlibraryDirectory=${library_directory}"}})
	o.Version.Arguments = &args
	repeated, err := PreviewOptions(o)
	if err != nil {
		t.Fatal(err)
	}
	count = 0
	for _, arg := range repeated.Arguments {
		if strings.HasPrefix(arg, "-DlibraryDirectory=") {
			count++
		}
	}
	if count != 1 {
		t.Fatal(count)
	}
}
func TestLaunchRejectsUnmanagedDirectoryAndClasspathOverrides(t *testing.T) {
	for _, name := range []string{"custom-directory", "metadata-directory", "double-directory", "bad-generation", "double-cp", "literal-cp", "property-cp", "alternate-cp"} {
		t.Run(name, func(t *testing.T) {
			o := lookupOptions(t)
			args := *o.Version.Arguments
			args.JVM = slices.Clone(args.JVM)
			o.Version.Arguments = &args
			switch name {
			case "custom-directory":
				o.JVMArguments = []string{"-DlibraryDirectory=/other"}
			case "metadata-directory":
				args.JVM = append(args.JVM, meta.Argument{Values: []string{"-DlibraryDirectory=/other"}})
			case "double-directory":
				args.JVM = append(args.JVM, meta.Argument{Values: []string{"-DlibraryDirectory=${library_directory}", "-DlibraryDirectory=${library_directory}"}})
			case "bad-generation":
				o.Version.LibraryDirectory = "cache/libraries"
			case "double-cp":
				args.JVM = append(args.JVM, meta.Argument{Values: []string{"-cp", "${classpath}"}})
			case "literal-cp":
				for i := range args.JVM {
					if slices.Contains(args.JVM[i].Values, "${classpath}") {
						args.JVM[i].Values = []string{"other.jar"}
					}
				}
			case "property-cp":
				args.JVM = append(args.JVM, meta.Argument{Values: []string{"-Djava.class.path=/other.jar"}})
			case "alternate-cp":
				args.JVM = append(args.JVM, meta.Argument{Values: []string{"--class-path=/other.jar"}})
			}
			if _, err := PreviewOptions(o); err == nil {
				t.Fatal("accepted uncontrolled launch path")
			}
		})
	}
}
