package fileio

import (
	"errors"
	"os"
	"strings"
)

// PublishDirectory atomically renames a prepared directory beside its final
// name. Both names are single components under the same open parent directory;
// the operation fails if any entry, including an empty directory, already exists.
func PublishDirectory(parent *os.Root, temporary, name string) error {
	return MoveNoReplace(parent, temporary, parent, name)
}

// MoveNoReplace atomically moves an entry between open directories on the same
// volume. Names are single components; directory traversal uses os.Root before
// the native call. Existing targets, including empty directories, are preserved.
func MoveNoReplace(source *os.Root, old string, target *os.Root, name string) error {
	for _, part := range []string{old, name} {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "/\\:\x00") {
			return errors.New("invalid publication name")
		}
	}
	return moveNoReplace(source, old, target, name)
}

// ReplaceManagedFile is for verified managed download/cache files only, never
// instance asset publication. Both parent handles are already identity-checked.
func ReplaceManagedFile(source *os.Root, old string, target *os.Root, name string) error {
	for _, part := range []string{old, name} {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "/\\:\x00") {
			return errors.New("invalid publication name")
		}
	}
	if info, err := source.Lstat(old); err != nil {
		return err
	} else if !info.Mode().IsRegular() {
		return errors.New("publication source is not a regular file")
	}
	if info, err := target.Lstat(name); err == nil && !info.Mode().IsRegular() {
		return errors.New("publication target is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return replaceManagedFile(source, old, target, name)
}
