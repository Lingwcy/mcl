package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"mcl/internal/fileio"
	launcherui "mcl/internal/ui"

	"github.com/egoist/mygo"
)

// Windows releases ship a single executable named per architecture, such as
// mcl-0.0.5-windows-amd64.exe, that runs wherever the user puts it. Update
// archives unpack mcl.exe and the resources next to it, and MyGo relaunches
// the executable the app started from. portableUpdates therefore downloads
// the update, then on restart (or quit) gives mcl.exe the running file's
// name, sets the old file aside for MyGo to delete at the next launch, and
// removes the resources the update added to the user's folder.

// archiveExe is the executable inside every Windows update archive.
const archiveExe = "mcl.exe"

// shippedResources are the files besides mcl.exe in update archives: the
// project's resources directory, which a test keeps in sync.
var shippedResources = []string{
	"font-awesome-LICENSE.txt",
	"go-toml-LICENSE.txt",
	"icon.png",
	"simple-icons-LICENSE.md",
}

const (
	portableCheckDelay    = 10 * time.Second
	portableCheckInterval = 24 * time.Hour
)

type portablePrefs struct {
	AutomaticChecks    bool      `json:"automatic_checks"`
	AutomaticDownloads bool      `json:"automatic_downloads"`
	LastCheck          time.Time `json:"last_check,omitzero"`
}

type portableUpdates struct {
	ctx       context.Context
	exe       string
	prefsPath string

	mu         sync.Mutex
	prefs      portablePrefs
	status     launcherui.UpdateStatus
	update     *mygo.Update
	staged     bool // mcl.exe waits next to a differently named executable
	invalidate func()
}

var _ launcherui.Updates = (*portableUpdates)(nil)

func newPortableUpdates(ctx context.Context, dataRoot string) (*portableUpdates, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	u := &portableUpdates{ctx: ctx, exe: exe, prefsPath: filepath.Join(dataRoot, "updates.json"), prefs: portablePrefs{AutomaticChecks: true}}
	if data, err := fileio.Read(ctx, u.prefsPath, 64<<10); err == nil {
		if err := json.Unmarshal(data, &u.prefs); err != nil {
			slog.Warn("update preferences are unreadable; using defaults", "error", err)
			u.prefs = portablePrefs{AutomaticChecks: true}
		}
	}
	return u, nil
}

// bind starts the background checks and redraws the shell on changes.
func (u *portableUpdates) bind(invalidate func()) {
	u.mu.Lock()
	u.invalidate = invalidate
	u.mu.Unlock()
	go func() {
		wait := portableCheckDelay
		for {
			select {
			case <-u.ctx.Done():
				return
			case <-time.After(wait):
			}
			wait = portableCheckInterval
			if u.AutomaticChecks() && u.Enabled() {
				u.check(true)
			}
		}
	}()
}

func (u *portableUpdates) notify() {
	u.mu.Lock()
	invalidate := u.invalidate
	u.mu.Unlock()
	if invalidate != nil {
		invalidate()
	}
}

func (u *portableUpdates) Enabled() bool { return mygo.Updater.Enabled() }

func (u *portableUpdates) Status() launcherui.UpdateStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

func (u *portableUpdates) Check() { u.check(false) }

func (u *portableUpdates) check(background bool) {
	u.mu.Lock()
	switch u.status.Phase {
	case launcherui.UpdateChecking, launcherui.UpdateDownloading, launcherui.UpdateReady:
		u.mu.Unlock()
		return
	}
	previous := u.status
	u.status = launcherui.UpdateStatus{Phase: launcherui.UpdateChecking}
	u.mu.Unlock()
	u.notify()
	go func() {
		up, err := mygo.Updater.Check(u.ctx)
		if u.ctx.Err() != nil {
			return
		}
		download := false
		u.mu.Lock()
		u.prefs.LastCheck = time.Now()
		switch {
		case err != nil && background:
			// A failed background check, such as offline, is not news.
			u.status = previous
			if u.status.Phase == launcherui.UpdateChecking {
				u.status = launcherui.UpdateStatus{}
			}
		case errors.Is(err, mygo.ErrUpdatesDisabled):
			u.status = launcherui.UpdateStatus{Phase: launcherui.UpdateFailed, Err: "当前构建不能自动更新"}
		case err != nil:
			u.status = launcherui.UpdateStatus{Phase: launcherui.UpdateFailed, Err: "检查更新失败：" + err.Error()}
		case up == nil:
			u.status = launcherui.UpdateStatus{Phase: launcherui.UpdateLatest}
		default:
			u.update = up
			u.status = launcherui.UpdateStatus{Phase: launcherui.UpdateAvailable, Version: up.Version, Notes: up.Notes}
			download = u.prefs.AutomaticDownloads
		}
		u.savePrefsLocked()
		u.mu.Unlock()
		u.notify()
		if download {
			u.Download()
		}
	}()
}

func (u *portableUpdates) Download() {
	u.mu.Lock()
	up := u.update
	if u.status.Phase != launcherui.UpdateAvailable || up == nil {
		u.mu.Unlock()
		return
	}
	dir := filepath.Dir(u.exe)
	if !isArchiveExe(u.exe) {
		if _, err := os.Lstat(filepath.Join(dir, archiveExe)); err == nil {
			u.status = launcherui.UpdateStatus{Phase: launcherui.UpdateFailed, Err: "同一文件夹中已有 mcl.exe，请把启动器放到单独的文件夹后再更新"}
			u.mu.Unlock()
			u.notify()
			return
		}
	}
	before := existingNames(dir)
	u.status = launcherui.UpdateStatus{Phase: launcherui.UpdateDownloading, Version: up.Version, Notes: up.Notes}
	u.mu.Unlock()
	u.notify()
	go func() {
		percent := -1
		err := up.Install(u.ctx, func(downloaded, total int64) {
			if total <= 0 {
				return
			}
			next := int(downloaded * 100 / total)
			u.mu.Lock()
			u.status.Progress = float64(downloaded) / float64(total)
			changed := next != percent
			percent = next
			u.mu.Unlock()
			if changed {
				u.notify()
			}
		})
		if u.ctx.Err() != nil {
			return
		}
		if err == nil {
			removeAddedResources(dir, before)
		}
		u.mu.Lock()
		if err != nil {
			u.status = launcherui.UpdateStatus{Phase: launcherui.UpdateFailed, Err: "下载更新失败：" + err.Error()}
		} else {
			u.staged = !isArchiveExe(u.exe)
			u.status = launcherui.UpdateStatus{Phase: launcherui.UpdateReady, Version: up.Version, Notes: up.Notes, Progress: 1}
		}
		u.mu.Unlock()
		u.notify()
	}()
}

func (u *portableUpdates) Restart() {
	u.mu.Lock()
	if u.status.Phase != launcherui.UpdateReady {
		u.mu.Unlock()
		return
	}
	if err := u.promoteLocked(); err != nil {
		u.status = launcherui.UpdateStatus{Phase: launcherui.UpdateFailed, Err: "安装更新失败：" + err.Error()}
		u.mu.Unlock()
		u.notify()
		return
	}
	u.mu.Unlock()
	mygo.App.Relaunch()
}

// finish installs a downloaded update as the app quits, so the next launch
// runs it.
func (u *portableUpdates) finish() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.promoteLocked(); err != nil {
		slog.Warn("downloaded update could not be installed", "error", err)
	}
}

func (u *portableUpdates) promoteLocked() error {
	if !u.staged {
		return nil
	}
	if err := promotePortable(u.exe, os.Getpid()); err != nil {
		return err
	}
	u.staged = false
	return nil
}

func (u *portableUpdates) AutomaticChecks() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.prefs.AutomaticChecks
}

func (u *portableUpdates) SetAutomaticChecks(on bool) {
	u.mu.Lock()
	u.prefs.AutomaticChecks = on
	u.savePrefsLocked()
	u.mu.Unlock()
	u.notify()
}

func (u *portableUpdates) AutomaticDownloads() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.prefs.AutomaticDownloads
}

func (u *portableUpdates) SetAutomaticDownloads(on bool) {
	u.mu.Lock()
	u.prefs.AutomaticDownloads = on
	u.savePrefsLocked()
	u.mu.Unlock()
	u.notify()
}

func (u *portableUpdates) LastCheck() time.Time {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.prefs.LastCheck
}

func (u *portableUpdates) savePrefsLocked() {
	data, err := json.Marshal(u.prefs)
	if err == nil {
		err = fileio.Write(context.WithoutCancel(u.ctx), u.prefsPath, data, 0o600)
	}
	if err != nil {
		slog.Warn("update preferences could not be saved", "error", err)
	}
}

func isArchiveExe(exe string) bool { return strings.EqualFold(filepath.Base(exe), archiveExe) }

// promotePortable gives the downloaded mcl.exe next to exe the name of exe.
// Windows lets a running executable be renamed but not deleted, so exe is
// set aside as .<name>.old-<pid>, which MyGo removes at the next launch.
func promotePortable(exe string, pid int) error {
	if isArchiveExe(exe) {
		return nil // the update already replaced it in place
	}
	dir := filepath.Dir(exe)
	fresh := filepath.Join(dir, archiveExe)
	if info, err := os.Lstat(fresh); err != nil {
		return fmt.Errorf("找不到已下载的新版本：%w", err)
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("%s 不是文件", fresh)
	}
	aside := filepath.Join(dir, "."+filepath.Base(exe)+".old-"+strconv.Itoa(pid))
	if err := os.Rename(exe, aside); err != nil {
		return err
	}
	if err := os.Rename(fresh, exe); err != nil {
		if undo := os.Rename(aside, exe); undo != nil {
			return errors.Join(err, undo)
		}
		return err
	}
	return nil
}

// existingNames lists the names in dir, lower case as Windows compares them.
func existingNames(dir string) map[string]bool {
	names := map[string]bool{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		names[strings.ToLower(e.Name())] = true
	}
	return names
}

// removeAddedResources deletes the resources that an update unpacked into
// dir where none were before, such as a Downloads folder; installs that
// shipped them keep theirs.
func removeAddedResources(dir string, before map[string]bool) {
	for _, name := range shippedResources {
		if before[strings.ToLower(name)] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("update resource could not be removed", "path", filepath.Join(dir, name), "error", err)
		}
	}
}
