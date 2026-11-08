package ui

import (
	"context"
	"errors"
	"slices"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/loader"
	"mcl/internal/core/market"

	"github.com/egoist/mygo/ui"
)

type creationState struct {
	mode                              int
	kind, loaderVersion, autoName     string
	versionOpen, loaderOpen, iconOpen bool
	kindOpen                          bool
	loaderQuery                       string
	busy                              bool
	err                               string
	generation                        uint64
	cancel                            func()
	releases                          []loader.Release
	cache                             map[string][]loader.Release
	loaderList                        ui.ListState
	iconStyle                         string
	icon                              []byte
	bitmap                            *ui.Bitmap
	iconBusy, importBusy, installNow  bool
	cancelIcon, cancelImport          func()
}

func (s *Shell) updateCreationName() {
	d, l := &s.creation, &s.library
	name := l.selected
	if d.kind != "" && name != "" {
		name += " · " + marketLoaderLabel(d.kind)
	}
	if l.name == "" || l.name == d.autoName || l.name == l.selected {
		l.name = name
	}
	d.autoName = name
}

func (s *Shell) selectCreationLoader(kind string) {
	if s.library.busy || kind == s.creation.kind {
		return
	}
	s.creation.kind = kind
	s.creation.loaderVersion, s.creation.loaderQuery = "", ""
	s.creation.loaderOpen = false
	s.updateCreationName()
	s.loadCreationReleases()
}

func (s *Shell) loadCreationReleases() {
	d := &s.creation
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
	}
	d.generation++
	d.busy, d.err, d.releases = false, "", nil
	kind, game := d.kind, s.library.selected
	if kind == "" || game == "" {
		return
	}
	if s.library.actions.LoaderReleases == nil {
		d.err = "加载器版本服务未配置"
		return
	}
	key := kind + ":" + game
	if rows, ok := d.cache[key]; ok {
		s.applyCreationReleases(rows)
		return
	}
	d.busy = true
	generation := d.generation
	d.cancel = s.library.actions.LoaderReleases(kind, game, func(rows []loader.Release, err error) {
		if generation != d.generation {
			return
		}
		d.busy, d.cancel = false, nil
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				d.err = err.Error()
			}
			return
		}
		if d.cache == nil {
			d.cache = make(map[string][]loader.Release)
		}
		if len(d.cache) >= 32 {
			clear(d.cache)
		}
		d.cache[key] = slices.Clone(rows)
		s.applyCreationReleases(rows)
	})
}

func (s *Shell) applyCreationReleases(rows []loader.Release) {
	d := &s.creation
	d.releases = slices.Clone(rows)
	if slices.ContainsFunc(rows, func(row loader.Release) bool { return row.Version == d.loaderVersion }) {
		return
	}
	d.loaderVersion = ""
	for _, row := range rows {
		if row.Recommended {
			d.loaderVersion = row.Version
			return
		}
	}
	for _, row := range rows {
		if row.Stable {
			d.loaderVersion = row.Version
			return
		}
	}
	if len(rows) > 0 {
		d.loaderVersion = rows[0].Version
	}
}

func (s *Shell) pickCreationIcon() {
	d := &s.creation
	if d.iconBusy || s.library.actions.PickIcon == nil {
		return
	}
	d.iconBusy = true
	d.cancelIcon = s.library.actions.PickIcon(func(data []byte, err error) {
		d.iconBusy, d.cancelIcon = false, nil
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				d.err = err.Error()
			}
			return
		}
		if len(data) == 0 {
			return
		}
		bitmap, err := ui.DecodeBitmap(data)
		if err != nil {
			d.err = "无法读取图标"
			return
		}
		d.icon, d.bitmap, d.iconStyle, d.iconOpen = data, bitmap, "", false
	})
}

func (s *Shell) pickCreationArchive() {
	d := &s.creation
	if d.importBusy || s.library.actions.PickArchive == nil {
		return
	}
	if s.market.install != nil && s.market.install.busy {
		d.err = "请先完成或取消当前安装任务"
		return
	}
	d.importBusy, d.err = true, ""
	d.cancelImport = s.library.actions.PickArchive(s.settings, func(plan app.ArchiveImportPlan, err error) {
		d.importBusy, d.cancelImport = false, nil
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				d.err = err.Error()
			}
			return
		}
		if plan.Instance.ID == "" {
			return
		}
		request := plan.Request
		s.market.install = &marketInstallState{
			project:     market.Project{Source: request.Source, Kind: market.Modpack, ID: request.ProjectID, Title: plan.Instance.Name},
			version:     market.Version{Source: request.Source, ProjectID: request.ProjectID, ID: request.VersionID, Name: plan.Manifest.Version},
			request:     market.InstallRequest{Source: request.Source, Kind: market.Modpack, ProjectID: request.ProjectID, VersionID: request.VersionID},
			archivePlan: &plan, archiveName: plan.Instance.Name, archiveMissing: plan.MissingRequired(), optional: make(map[string]bool), localArchive: request.LocalArchive,
		}
		s.market.showInstall = true
	})
}

func (s *Shell) creationReady() bool {
	d, l := &s.creation, &s.library
	return !l.busy && !d.busy && !d.iconBusy && l.selected != "" && instance.ValidateName(strings.TrimSpace(l.name)) == nil &&
		(d.kind == "" || (d.loaderVersion != "" && d.err == ""))
}

func (s *Shell) cacheInstanceIcons(values []instance.Instance) {
	l := &s.library
	if l.icons == nil {
		l.icons = make(map[string]*ui.Bitmap)
	}
	for _, value := range values {
		if len(value.Icon) == 0 {
			delete(l.icons, value.ID)
			continue
		}
		if bitmap, err := ui.DecodeBitmap(value.Icon); err == nil {
			l.icons[value.ID] = bitmap
		}
	}
}
