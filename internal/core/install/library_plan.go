package install

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/download"
	"mcl/internal/core/meta"
)

type LibraryRole string

const (
	LibraryClasspath       LibraryRole = "classpath"
	LibraryNativeClasspath LibraryRole = "native-classpath"
	LibraryNativeArchive   LibraryRole = "native-archive"
	LibraryRuntimeLookup   LibraryRole = "runtime-lookup"
)

// LibraryFile is one selected role in the final effective library set. It is
// built AFTER rules/OS/CPU selection; declarations that do not match the target
// are never allowed to register a conflicting path or override an earlier file.
// Modern native JARs stay on the classpath, extracted legacy natives do not.
type LibraryFile struct {
	Coordinate string
	File       download.File
	Role       LibraryRole
	Local      bool
	Exclude    []string
}

type libraryRoleKey struct {
	path string
	role LibraryRole
}

type librarySelection struct {
	plan       *Plan
	addRemote  func(string, meta.Download) (download.File, error)
	identities map[string]LibraryFile
	entries    map[libraryRoleKey]int
}

func newLibrarySelection(plan *Plan, add func(string, meta.Download) (download.File, error)) *librarySelection {
	return &librarySelection{plan: plan, addRemote: add, identities: make(map[string]LibraryFile), entries: make(map[libraryRoleKey]int)}
}
func (s *librarySelection) add(value LibraryFile) error {
	if _, err := meta.ParseCoordinate(value.Coordinate); err != nil {
		return err
	}
	if !archive.SafePath(value.File.Path) || value.File.Download.ValidateIdentity() != nil || value.File.Size > download.MaxFileSize {
		return errors.New("选中的运行库文件身份无效")
	}
	if value.Local && value.File.URL != "" {
		return errors.New("本地运行库不能携带下载地址")
	}
	if value.Role == LibraryRuntimeLookup && !value.Local {
		return errors.New("运行库查询不能转换为远程下载")
	}
	key := archive.Key(value.File.Path)
	if previous, ok := s.identities[key]; ok {
		if previous.File.Path != value.File.Path || previous.File.Size != value.File.Size || !strings.EqualFold(previous.File.SHA1, value.File.SHA1) || previous.Local != value.Local || previous.Coordinate != value.Coordinate {
			return fmt.Errorf("运行库目标身份冲突: %s", value.Coordinate)
		}
	}
	entryKey := libraryRoleKey{path: key, role: value.Role}
	if index, ok := s.entries[entryKey]; ok {
		if !slices.Equal(s.plan.Libraries[index].Exclude, value.Exclude) {
			return errors.New("同一本地库归档存在不同的解压排除项")
		}
		return nil
	}
	switch value.Role {
	case LibraryClasspath, LibraryNativeClasspath, LibraryNativeArchive, LibraryRuntimeLookup:
	default:
		return errors.New("运行库角色无效")
	}
	if !value.Local {
		file, err := s.addRemote(value.File.Path, value.File.Download)
		if err != nil {
			return err
		}
		value.File = file
	}
	value.Exclude = slices.Clone(value.Exclude)
	value.File.AllowedHosts = slices.Clone(value.File.AllowedHosts)
	s.identities[key] = value
	s.entries[entryKey] = len(s.plan.Libraries)
	s.plan.Libraries = append(s.plan.Libraries, value)
	switch value.Role {
	case LibraryClasspath, LibraryNativeClasspath:
		if !slices.Contains(s.plan.Classpath, value.File.Path) {
			s.plan.Classpath = append(s.plan.Classpath, value.File.Path)
		}
		if value.Local && !slices.ContainsFunc(s.plan.LocalLibraries, func(file download.File) bool { return file.Path == value.File.Path }) {
			s.plan.LocalLibraries = append(s.plan.LocalLibraries, value.File)
		}
		if value.Role == LibraryNativeClasspath {
			s.plan.NativeJARs = append(s.plan.NativeJARs, value.File)
		}
	case LibraryNativeArchive:
		s.plan.Natives = append(s.plan.Natives, NativeArchive{File: value.File, Exclude: slices.Clone(value.Exclude)})
	case LibraryRuntimeLookup:
		s.plan.RuntimeLookups = append(s.plan.RuntimeLookups, value.File)
	}
	return nil
}

// LWJGL2 and JInput publish native-distribution containers as separate Maven
// artifacts. Their unclassified packaging JAR is not the Java runtime API.
// Hybrid libraries (text2speech/ObjC bridge) are deliberately NOT in this list.
func legacyNativeContainer(coordinate meta.Coordinate, library meta.Library) bool {
	if coordinate.Classifier != "" || coordinate.Extension != "jar" || len(library.Natives) == 0 {
		return false
	}
	known := coordinate.Group == "org.lwjgl.lwjgl" && coordinate.Artifact == "lwjgl-platform" || coordinate.Group == "net.java.jinput" && coordinate.Artifact == "jinput-platform"
	if !known {
		return false
	}
	if file := library.Downloads.Artifact; file != nil {
		// This exact digest is the 22-byte empty ZIP, observed in the fixed
		// official 1.12.2 document. Unknown/nonempty bytes are not discarded
		// merely because the Maven artifact has a familiar name.
		return file.Size == 22 && strings.EqualFold(file.SHA1, "b04f3ee8f5e43fa3b162981b50bb72fe1acabb33")
	}
	return true
}
