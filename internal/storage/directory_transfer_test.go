package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/launch"
	"mcl/internal/core/meta"
	"mcl/internal/core/transfer"
)

func directoryWrite(t *testing.T, root, name, value string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}
func directoryValue() instance.Instance {
	return instance.Instance{ID: instance.NewID(), Name: "目录回归", RequiredJava: 17, CreatedAt: time.Now().UTC(), Version: meta.ManifestEntry{ID: "1.20.4", Type: "release", URL: "https://example.com/version.json", SHA1: strings.Repeat("0", 40)}}
}
func directoryFixture(t *testing.T) (string, instance.Instance, app.GameDirectory, *DirectoryTransferStore, *SessionStore, *app.DirectoryService) {
	t.Helper()
	root := t.TempDir()
	location, err := NewGameDirectoryStore(root).Add(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := directoryValue()
	if err := NewInstanceStore(filepath.Join(root, "instances")).Create(t.Context(), value); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{"game/saves/world/level.dat": "world", "game/mods/local.jar": "mod", "removed-assets/old/payload": "removed", "asset-updates/old/payload": "old-version"} {
		directoryWrite(t, filepath.Join(root, "instances", value.ID), path, data)
	}
	directoryWrite(t, root, "cache/shared", "keep")
	directoryWrite(t, filepath.Join(root, "instances", value.ID), "runtime/keep", "central")
	store := NewDirectoryTransferStore(root)
	sessions := NewSessionStore(root, func(launch.ProcessIdentity) (bool, error) { return false, nil })
	trashRoot := t.TempDir()
	calls := 0
	trash := func(name string) error {
		calls++
		return os.Rename(name, filepath.Join(trashRoot, strconv.Itoa(calls)))
	}
	return root, value, location, store, sessions, app.NewDirectoryService(store, sessions, nil, nil, trash)
}
func TestDirectoryMoveKeepsIdentitySharedStateAndRecoverableBackup(t *testing.T) {
	root, value, location, store, _, service := directoryFixture(t)
	plan, err := service.InspectMove(t.Context(), value.ID, location.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := service.Run(t.Context(), plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if moved.ID != value.ID || moved.GameDirectoryID != location.ID {
		t.Fatal("identity or location changed incorrectly")
	}
	for _, name := range []string{"game/saves/world/level.dat", "game/mods/local.jar", "removed-assets/old/payload", "asset-updates/old/payload"} {
		if _, err := os.Stat(filepath.Join(location.Path, value.ID, name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"cache/shared", filepath.Join("instances", value.ID, "runtime/keep")} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatal("central/shared data moved", err)
		}
	}
	operations, err := store.ListOperations(t.Context())
	if err != nil || len(operations) != 1 || !operations[0].Backup {
		t.Fatalf("missing retained backup: %+v %v", operations, err)
	}
	backup, err := store.BackupDirectory(t.Context(), value.ID, plan.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(backup, "game/saves/world/level.dat")); err != nil {
		t.Fatal("only source copy lost", err)
	}
	// Both directions use copy/verify/commit; the original central game slot is
	// free, while the earlier backup remains a separate protected sibling.
	back, err := service.InspectMove(t.Context(), value.ID, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Run(t.Context(), back, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "instances", value.ID, "game/saves/world/level.dat")); err != nil {
		t.Fatal(err)
	}
	deletion, err := NewInstanceDeletionStore(root).Inspect(t.Context(), value.ID)
	if err != nil || len(deletion.MigrationBackups) != 2 {
		t.Fatalf("deletion omitted old payloads: %+v %v", deletion, err)
	}
}
func TestDirectoryCancellationBlocksNormalRecoveryAndCanResume(t *testing.T) {
	root, value, location, store, sessions, service := directoryFixture(t)
	plan, err := service.InspectMove(t.Context(), value.ID, location.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	_, err = service.Run(ctx, plan, func(p transfer.Progress) {
		if p.Files > 0 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, reserve := range []func(context.Context, string) (app.SessionLease, error){sessions.Reserve, sessions.ReserveForRecovery, sessions.ReserveForDeletion} {
		if lease, err := reserve(t.Context(), value.ID); err == nil {
			lease.Close()
			t.Fatal("pending directory operation bypassed")
		}
	}
	if _, err := os.Stat(filepath.Join(root, "instances", value.ID, "game/saves/world/level.dat")); err != nil {
		t.Fatal("source removed by cancellation", err)
	}
	if _, err := service.Resume(t.Context(), value.ID, instance.NewID(), nil); err == nil {
		t.Fatal("obsolete transaction token accepted")
	}
	if _, err := service.Resume(t.Context(), value.ID, plan.Token, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pending(t.Context(), value.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}
func TestDirectoryChangedSourceAndUnownedTargetArePreservedOnAbort(t *testing.T) {
	root, value, location, store, _, service := directoryFixture(t)
	plan, err := service.InspectMove(t.Context(), value.ID, location.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	directoryWrite(t, filepath.Join(root, "instances", value.ID), "game/mods/local.jar", "new-source")
	if _, err := service.Run(t.Context(), plan, nil); err == nil {
		t.Fatal("source edit accepted")
	}
	directoryWrite(t, filepath.Join(location.Path, value.ID), "conflict.txt", "not ours")
	if err := service.Abort(t.Context(), value.ID, plan.Token); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{filepath.Join(root, "instances", value.ID, "game/mods/local.jar"): "new-source", filepath.Join(location.Path, value.ID, "conflict.txt"): "not ours"} {
		data, err := os.ReadFile(name)
		if err != nil || string(data) != want {
			t.Fatalf("unrelated data altered: %s %v", name, err)
		}
	}
	if _, err := store.Pending(t.Context(), value.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("abort not completed", err)
	}
}
func TestDirectoryLostStageRestartsOnlyFromFixedSource(t *testing.T) {
	_, value, location, store, _, service := directoryFixture(t)
	plan, err := service.InspectMove(t.Context(), value.ID, location.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Begin(t.Context(), plan); err != nil {
		t.Fatal(err)
	}
	// Simulate a persisted publishing checkpoint with the volume's earlier
	// staging entry absent. No real process, power interruption or disk is used.
	if err := store.writeState(t.Context(), directoryState{Schema: 1, Plan: plan, Phase: "publishing"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resume(t.Context(), value.ID, plan.Token, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(location.Path, value.ID, "game/saves/world/level.dat")); err != nil {
		t.Fatal(err)
	}
}
func TestDirectoryCommittedMissingFilesUseOldCopyWithoutOverwriting(t *testing.T) {
	_, value, location, store, _, service := directoryFixture(t)
	plan, err := service.InspectMove(t.Context(), value.ID, location.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Begin(t.Context(), plan); err != nil {
		t.Fatal(err)
	}
	parent, err := store.parent(t.Context(), location.ID, value.ID)
	if err != nil {
		t.Fatal(err)
	}
	stage, payload, err := store.stage(t.Context(), parent, plan)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.payload(t.Context(), "", value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := transfer.Copy(t.Context(), source, payload, plan.Tree, nil); err != nil {
		t.Fatal(err)
	}
	source.Close()
	for _, part := range planParts(plan) {
		node, err := openRealDirectory(payload, part)
		if err != nil {
			t.Fatal(err)
		}
		err = stampDirectory(t.Context(), node, transfer.OwnerFile, plan)
		node.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	payload.Close()
	stage.Close()
	if err := store.publish(t.Context(), parent, plan); err != nil {
		t.Fatal(err)
	}
	parent.Close()
	if err := store.commitLocation(t.Context(), plan); err != nil {
		t.Fatal(err)
	}
	if err := store.writeState(t.Context(), directoryState{Schema: 1, Plan: plan, Phase: "committed"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(location.Path, value.ID, "game/saves/world/level.dat")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resume(t.Context(), value.ID, plan.Token, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(location.Path, value.ID, "game/saves/world/level.dat"))
	if err != nil || string(data) != "world" {
		t.Fatal("fixed file not restored", err)
	}
}
func TestDirectoryImportNeverAdoptsForeignRuntimeOrAccounts(t *testing.T) {
	root, foreign := t.TempDir(), t.TempDir()
	directoryWrite(t, foreign, "versions/1.20.4/1.20.4.json", `{"id":"1.20.4","mainClass":"net.minecraft.client.main.Main"}`)
	for name, data := range map[string]string{"saves/world/level.dat": "world", "mods/local.jar": "mod", "config/normal.json": "setting", "config/accounts.json": "secret", "launcher_profiles.json": "private", "start.sh": "script", "versions/1.20.4/1.20.4.jar": "untrusted-client"} {
		directoryWrite(t, foreign, name, data)
	}
	sources := NewExistingGamesStore(root)
	source, err := sources.Add(t.Context(), foreign)
	if err != nil {
		t.Fatal(err)
	}
	report, err := sources.Scan(t.Context(), source.ID)
	if err != nil || len(report.Candidates) != 1 {
		t.Fatalf("fixture discovery: %+v %v", report, err)
	}
	request := app.DirectoryImportRequest{SourceID: source.ID, CandidateID: report.Candidates[0].ID, DataPath: source.Path, Name: "受管副本", Groups: []string{"saves", "mods", "config"}}
	store := NewDirectoryTransferStore(root)
	_, fingerprint, tree, err := store.InspectSource(t.Context(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	value := directoryValue()
	value.Name = request.Name
	plan := app.DirectoryPlan{Kind: "import", Token: instance.NewID(), SourceID: source.ID, CandidateID: request.CandidateID, DataPath: request.DataPath, SourceFingerprint: fingerprint, After: value, Tree: tree}
	if err := store.Begin(t.Context(), plan); err != nil {
		t.Fatal(err)
	}
	if err := sources.Remove(t.Context(), source.ID); err == nil {
		t.Fatal("pending source registration removed")
	}
	sessions := NewSessionStore(root, func(launch.ProcessIdentity) (bool, error) { return false, nil })
	lease, err := sessions.ReserveForDirectoryTransfer(t.Context(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Run(t.Context(), plan, nil)
	lease.Close()
	if err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(root, "instances", value.ID, "game")
	for _, name := range []string{"saves/world/level.dat", "mods/local.jar", "config/normal.json"} {
		if _, err := os.Stat(filepath.Join(game, name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"config/accounts.json", "launcher_profiles.json", "start.sh", "versions/1.20.4/1.20.4.jar"} {
		if _, err := os.Stat(filepath.Join(game, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("private/runtime adopted: %s", name)
		}
		if _, err := os.Stat(filepath.Join(foreign, name)); err != nil {
			t.Fatal("foreign file removed", err)
		}
	}
}
func TestDirectoryPreviewHonorsInstanceLockAndJournalScope(t *testing.T) {
	_, value, location, store, sessions, service := directoryFixture(t)
	lease, err := sessions.Reserve(t.Context(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.InspectMove(t.Context(), value.ID, location.ID, nil); err == nil {
		t.Fatal("occupied instance accepted")
	}
	lease.Close()
	plan, err := service.InspectMove(t.Context(), value.ID, location.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	bad := plan
	bad.After.Name = "not a directory change"
	if err := store.Begin(t.Context(), bad); err == nil {
		t.Fatal("migration changed unrelated metadata")
	}
	bad = plan
	bad.Tree.Roots = []string{"../escape"}
	if err := store.Begin(t.Context(), bad); err == nil {
		t.Fatal("unsafe journal accepted")
	}
}

func TestDirectoryOfflineTargetKeepsPendingAndOriginal(t *testing.T) {
	root, value, location, store, _, service := directoryFixture(t)
	plan, err := service.InspectMove(t.Context(), value.ID, location.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Begin(t.Context(), plan); err != nil {
		t.Fatal(err)
	}
	offline := location.Path + "-offline"
	if err := os.Rename(location.Path, offline); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resume(t.Context(), value.ID, plan.Token, nil); err == nil {
		t.Fatal("offline volume silently succeeded")
	}
	if _, err := os.Stat(location.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing volume was recreated")
	}
	if _, err := os.Stat(filepath.Join(root, "instances", value.ID, "game/saves/world/level.dat")); err != nil {
		t.Fatal("offline failure lost source", err)
	}
	if _, err := store.Pending(t.Context(), value.ID); err != nil {
		t.Fatal("offline failure removed recovery record", err)
	}
	if err := os.Rename(offline, location.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resume(t.Context(), value.ID, plan.Token, nil); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryBackupCleanupPreservesUnexpectedChanges(t *testing.T) {
	_, value, location, store, _, service := directoryFixture(t)
	plan, err := service.InspectMove(t.Context(), value.ID, location.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Run(t.Context(), plan, nil); err != nil {
		t.Fatal(err)
	}
	name, err := store.BackupDirectory(t.Context(), value.ID, plan.Token)
	if err != nil {
		t.Fatal(err)
	}
	directoryWrite(t, name, "game/saves/late-world/level.dat", "new file after migration")
	calls := 0
	err = store.CleanBackup(t.Context(), value.ID, plan.Token, func(string) error { calls++; return nil })
	if err == nil || calls != 0 {
		t.Fatal("changed backup passed through to trash")
	}
	if _, err := os.Stat(filepath.Join(name, "game/saves/late-world/level.dat")); err != nil {
		t.Fatal("changed backup removed", err)
	}
	if err := NewGameDirectoryStore(store.root).Remove(t.Context(), location.ID); err == nil {
		t.Fatal("active location removed")
	}
}
