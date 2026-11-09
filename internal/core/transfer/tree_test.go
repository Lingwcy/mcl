package transfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func transferFile(t *testing.T, root, name, content string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func transferRoot(t *testing.T, name string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return root
}
func TestImportCopiesOnlyApprovedFilesAndKeepsOriginal(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	for name, value := range map[string]string{"saves/world/level.dat": "world", "mods/example.jar": "mod", "config/normal.json": "setting", "config/accounts.json": "secret", "config/.env": "secret", "config/start.sh": "script", "saves/world/session.lock": "lock", "launcher_profiles.json": "private", "versions/custom/custom.json": "foreign arguments"} {
		transferFile(t, source, name, value)
	}
	src, dst := transferRoot(t, source), transferRoot(t, target)
	tree, err := Scan(t.Context(), src, []string{"saves", "mods", "config"}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Files) != 3 || len(tree.Skipped) != 4 {
		t.Fatalf("unexpected copy scope: %+v", tree)
	}
	if err := Copy(t.Context(), src, dst, tree, nil); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCopied(t.Context(), dst, tree); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config/accounts.json", "config/.env", "config/start.sh", "saves/world/session.lock", "launcher_profiles.json", "versions/custom/custom.json"} {
		if _, err := os.Stat(filepath.Join(target, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("private file copied: %s", name)
		}
		if _, err := os.Stat(filepath.Join(source, name)); err != nil {
			t.Fatalf("source changed: %s", name)
		}
	}
}
func TestCopyRejectsSourceEditsAndPreservesConflictingDestination(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	transferFile(t, source, "game/mods/a.jar", "original")
	src, dst := transferRoot(t, source), transferRoot(t, target)
	tree, err := Scan(t.Context(), src, []string{"game"}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	transferFile(t, source, "game/mods/a.jar", "changed")
	if err := Copy(t.Context(), src, dst, tree, nil); err == nil {
		t.Fatal("source change accepted")
	}
	transferFile(t, target, "game/mods/a.jar", "keep")
	if err := Copy(t.Context(), src, dst, tree, nil); err == nil {
		t.Fatal("destination conflict accepted")
	}
	data, _ := os.ReadFile(filepath.Join(target, "game/mods/a.jar"))
	if string(data) != "keep" {
		t.Fatal("destination overwritten")
	}
}
func TestTransferRejectsLinksAliasesAndDuplicateRoots(t *testing.T) {
	source := t.TempDir()
	transferFile(t, source, "game/a", "one")
	src := transferRoot(t, source)
	if _, err := Scan(t.Context(), src, []string{"game", "game"}, false, nil); err == nil {
		t.Fatal("duplicate roots accepted")
	}
	transferFile(t, source, "game/A", "two")
	// Case-insensitive filesystems expose one physical entry; distinct aliases
	// are asserted only where both actual names can coexist.
	entries, _ := os.ReadDir(filepath.Join(source, "game"))
	if len(entries) == 2 {
		if _, err := Scan(t.Context(), src, []string{"game"}, false, nil); err == nil {
			t.Fatal("case aliases accepted")
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(source, "game/link")); err != nil {
		t.Skip("symlink unavailable", err)
	}
	if _, err := Scan(t.Context(), src, []string{"game"}, false, nil); err == nil {
		t.Fatal("link accepted")
	}
}
func TestTransferCancellationAndCopiedTimestampTolerance(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	transferFile(t, source, "game/saves/world/level.dat", "world")
	src, dst := transferRoot(t, source), transferRoot(t, target)
	tree, err := Scan(t.Context(), src, []string{"game"}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Copy(canceled, src, dst, tree, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := Copy(t.Context(), src, dst, tree, nil); err != nil {
		t.Fatal(err)
	}
	// FAT/Windows and Unix have different timestamp precision. Content identity
	// remains exact; copied metadata precision must not invalidate safe bytes.
	name := filepath.Join(target, "game/saves/world/level.dat")
	stamp := time.Now().Truncate(2 * time.Second)
	if err := os.Chtimes(name, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCopied(t.Context(), dst, tree); err != nil {
		t.Fatal(err)
	}
	if err := Verify(t.Context(), dst, tree, false); err == nil {
		t.Fatal("source snapshot incorrectly ignored metadata changes")
	}
}
