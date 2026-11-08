package ui

import (
	"testing"

	"mcl/internal/app"
	"mcl/internal/core/instance"

	"github.com/egoist/mygo/ui"
)

func TestInstancePreferencesCoalesceWithoutPublishingUncommittedDrafts(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	original := instance.Instance{ID: instance.NewID(), Name: "original"}
	shell.library.snapshot.Instances = []instance.Instance{original}
	shell.library.selectedID = original.ID
	shell.library.actions.LoadInstance = func(_ string, done func(instance.Instance, error)) { done(original, nil) }
	type write struct {
		expected, desired app.InstancePreferences
		done              func(instance.Instance, error)
	}
	var writes []write
	shell.library.actions.SavePreferences = func(_ string, expected, desired app.InstancePreferences, done func(instance.Instance, error)) {
		writes = append(writes, write{expected, desired, done})
	}
	shell.openInstanceSettings()
	state := shell.instanceEditor
	state.name = "first"
	shell.saveInstancePreferences(state)
	state.name = "latest"
	shell.saveInstancePreferences(state)
	if len(writes) != 1 || shell.library.snapshot.Instances[0].Name != "original" {
		t.Fatal("an in-flight edit was duplicated or an unsaved draft leaked to the instance list")
	}
	first := original
	first.Name = "first"
	writes[0].done(first, nil)
	if len(writes) != 2 || writes[1].expected.Name != "first" || writes[1].desired.Name != "latest" || state.name != "latest" {
		t.Fatal("the newest draft was lost or did not compare against the committed write")
	}
	shell.page = libraryPage
	latest := original
	latest.Name = "latest"
	writes[1].done(latest, nil)
	if state.saving || shell.library.snapshot.Instances[0].Name != "latest" || state.base.Name != "latest" {
		t.Fatal("navigation discarded the completion of an accepted edit")
	}
}

func TestInstancePreferencesRejectStaleLoadAndRequireConflictReload(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	first := instance.Instance{ID: instance.NewID(), Name: "first"}
	second := instance.Instance{ID: instance.NewID(), Name: "second"}
	shell.library.snapshot.Instances = []instance.Instance{first, second}
	var loads []func(instance.Instance, error)
	shell.library.actions.LoadInstance = func(_ string, done func(instance.Instance, error)) { loads = append(loads, done) }
	shell.library.selectedID = first.ID
	shell.openInstanceSettings()
	shell.library.selectedID = second.ID
	shell.openInstanceSettings()
	loads[0](first, nil)
	if shell.instanceEditor.loaded || shell.instanceEditor.id != second.ID {
		t.Fatal("an obsolete load replaced the newly selected editor")
	}
	loads[1](second, nil)
	var saves int
	shell.library.actions.SavePreferences = func(_ string, _, _ app.InstancePreferences, done func(instance.Instance, error)) {
		saves++
		done(instance.Instance{}, app.ErrInstanceSettingsChanged)
	}
	state := shell.instanceEditor
	state.name = "my edit"
	shell.saveInstancePreferences(state)
	state.name = "another edit"
	shell.saveInstancePreferences(state)
	if saves != 1 || !state.conflict || shell.library.snapshot.Instances[1].Name != "second" {
		t.Fatal("a conflicting edit was retried or published without a reload")
	}
	shell.loadInstanceSettings(second)
	second.Name = "external edit"
	loads[2](second, nil)
	if shell.instanceEditor.conflict || shell.instanceEditor.name != "external edit" {
		t.Fatal("reload did not adopt the authoritative external edit")
	}
}

func TestInstanceAutomaticIconRemovesCachedCustomArtwork(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	original := instance.Instance{ID: instance.NewID(), Name: "original", Icon: []byte("previous icon")}
	state := &instanceSettingsState{id: original.ID, name: original.Name, icon: original.Icon, base: app.PreferencesForInstance(original), value: original, loaded: true}
	shell.library.icons = map[string]*ui.Bitmap{original.ID: instancePresets["grass"]}
	shell.library.snapshot.Instances = []instance.Instance{original}
	shell.library.actions.SavePreferences = func(_ string, _, desired app.InstancePreferences, done func(instance.Instance, error)) {
		updated := original
		updated.Icon, updated.IconStyle = desired.Icon, desired.IconStyle
		done(updated, nil)
	}
	shell.chooseInstanceSettingsIcon(state, "")
	if _, cached := shell.library.icons[original.ID]; cached || len(state.base.Icon) != 0 {
		t.Fatal("automatic icon kept the previous custom bitmap")
	}
}

func TestInstanceSwitchCancelsAndRejectsPreviousIconChoice(t *testing.T) {
	shell := NewShell(app.DefaultSettings(), t.TempDir(), nil, nil)
	first := instance.Instance{ID: instance.NewID(), Name: "first"}
	second := instance.Instance{ID: instance.NewID(), Name: "second"}
	shell.library.snapshot.Instances = []instance.Instance{first, second}
	shell.library.selectedID = first.ID
	shell.library.actions.LoadInstance = func(_ string, done func(instance.Instance, error)) { done(first, nil) }
	var completion func([]byte, error)
	var canceled bool
	shell.library.actions.PickIcon = func(done func([]byte, error)) func() {
		completion = done
		return func() { canceled = true }
	}
	shell.openInstanceSettings()
	state := shell.instanceEditor
	shell.pickInstanceSettingsIcon(state)
	shell.selectInstance(second.ID)
	completion([]byte("obsolete image result"), nil)
	if !canceled || state.iconBusy || len(state.icon) != 0 || state.saveError != "" {
		t.Fatal("switching instances kept the old picker or applied its stale result")
	}
}
