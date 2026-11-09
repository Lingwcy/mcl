package main

import (
	"runtime"
	"time"

	launcherui "mcl/internal/ui"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
)

// updatePlugin draws the macOS update window in native UI. Releases are
// verified against updates.publicKey in mygo.json. Windows ships a portable
// executable, which portableUpdates updates instead (see update_portable.go).
var updatePlugin = native.New(updater.Options{Language: "zh-Hans"})

// usesUpdatePlugin reports whether this platform updates through the plugin's
// window. The plugin relaunches the executable it started from, which on
// Windows keeps the old portable file when it is not named mcl.exe.
func usesUpdatePlugin() bool { return runtime.GOOS != "windows" }

// pluginUpdates adapts the MyGo updater plugin to the shell's about page.
// The plugin's window owns checking, downloading and relaunching.
type pluginUpdates struct{}

var _ launcherui.Updates = pluginUpdates{}

func (pluginUpdates) Enabled() bool { return mygo.Updater.Enabled() }
func (pluginUpdates) Check()        { updater.CheckForUpdates() }
func (pluginUpdates) Download()     { updater.CheckForUpdates() }
func (pluginUpdates) Restart()      { updater.CheckForUpdates() }
func (pluginUpdates) Status() launcherui.UpdateStatus {
	return launcherui.UpdateStatus{Phase: launcherui.UpdateIdle, Version: updater.AvailableVersion()}
}
func (pluginUpdates) AutomaticChecks() bool         { return updater.AutomaticChecks() }
func (pluginUpdates) SetAutomaticChecks(on bool)    { updater.SetAutomaticChecks(on) }
func (pluginUpdates) AutomaticDownloads() bool      { return updater.AutomaticDownloads() }
func (pluginUpdates) SetAutomaticDownloads(on bool) { updater.SetAutomaticDownloads(on) }
func (pluginUpdates) LastCheck() time.Time          { return updater.LastCheck() }

// connectUpdates shows the version and updater state in the shell and
// redraws it whenever they change.
func connectUpdates(window *mygo.Window, shell *launcherui.Shell, portable *portableUpdates) {
	if portable != nil {
		portable.bind(window.Invalidate)
		shell.BindUpdates(mygo.App.Version(), portable)
		return
	}
	shell.BindUpdates(mygo.App.Version(), pluginUpdates{})
	// Subscribed for the application's lifetime, like its single window.
	updater.OnChange(window.Invalidate)
}
