package main

import (
	"time"

	launcherui "mcl/internal/ui"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
)

// updatePlugin draws the update window in native UI, so Windows needs no
// WebView2 runtime. Releases are verified against updates.publicKey in mygo.json.
var updatePlugin = native.New(updater.Options{Language: "zh-Hans"})

// selfUpdates adapts the MyGo updater to the shell's about page.
type selfUpdates struct{}

var _ launcherui.Updates = selfUpdates{}

func (selfUpdates) Enabled() bool                 { return mygo.Updater.Enabled() }
func (selfUpdates) Check()                        { updater.CheckForUpdates() }
func (selfUpdates) AutomaticChecks() bool         { return updater.AutomaticChecks() }
func (selfUpdates) SetAutomaticChecks(on bool)    { updater.SetAutomaticChecks(on) }
func (selfUpdates) AutomaticDownloads() bool      { return updater.AutomaticDownloads() }
func (selfUpdates) SetAutomaticDownloads(on bool) { updater.SetAutomaticDownloads(on) }
func (selfUpdates) LastCheck() time.Time          { return updater.LastCheck() }
func (selfUpdates) AvailableVersion() string      { return updater.AvailableVersion() }

// connectUpdates shows the version and updater state in the shell and
// redraws it whenever a check or the update window changes them.
func connectUpdates(window *mygo.Window, shell *launcherui.Shell) {
	shell.BindUpdates(mygo.App.Version(), selfUpdates{})
	// Subscribed for the application's lifetime, like its single window.
	updater.OnChange(window.Invalidate)
}
