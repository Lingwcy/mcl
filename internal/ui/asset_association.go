package ui

import (
	"context"
	"errors"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/market"

	"github.com/egoist/mygo/ui"
)

type assetAssociationState struct {
	instanceID string
	file       app.AssetFile
	origin     *market.InstallFile
	active     bool
	cancel     func()
	message    string
	failed     bool
}

func (s *Shell) marketAssociation() *assetAssociationState {
	m := &s.market
	if a := m.association; m.showAssociation && a != nil && a.file.Kind == m.kind {
		return a
	}
	return nil
}

func (s *Shell) associatingAsset(id string) bool {
	state := s.market.association
	return state != nil && state.active && state.instanceID == id
}

func (s *Shell) openAssetAssociation(id string, file app.AssetFile) {
	if s.assets.actions.Associate == nil || id == "" || file.Directory {
		return
	}
	m := &s.market
	if m.association == nil || !m.association.active {
		m.association = &assetAssociationState{instanceID: id, file: file}
	}
	if !m.actions.CurseForgeAvailable || m.source == "" {
		m.source = market.Modrinth
	}
	m.text, m.targetID, m.gameVersion = "", "", ""
	m.resetFilters()
	s.showAssetAssociation(m.association)
}

func (s *Shell) showAssetAssociation(state *assetAssociationState) {
	m := &s.market
	if state == nil || m.association != state {
		return
	}
	m.kind, s.page, s.sidebarCollapsed = state.file.Kind, marketPage, false
	m.showKey, m.showInstall, m.showAssociation = false, false, true
	s.loadMarketCategories()
	s.searchMarket(true)
}

func (s *Shell) startAssetAssociation(state *assetAssociationState, source market.Source, project, version string) {
	if state == nil || s.market.association != state || state.active || state.origin != nil ||
		s.assets.actions.Associate == nil || (source == market.CurseForge && !s.market.actions.CurseForgeAvailable) {
		return
	}
	state.active, state.failed, state.message = true, false, ""
	request := app.AssetAssociation{InstanceID: state.instanceID, File: state.file, Source: source, ProjectID: project, VersionID: version}
	state.cancel = s.assets.actions.Associate(request, func(origin market.InstallFile, err error) {
		if s.market.association != state {
			return
		}
		state.active, state.cancel = false, nil
		switch {
		case errors.Is(err, context.Canceled):
			state.message = "已取消"
		case err != nil:
			state.message, state.failed = err.Error(), true
		default:
			state.origin = &origin
			state.message = "已关联 · " + string(origin.Source)
			if origin.Metadata != nil && origin.Metadata.Title != "" {
				state.message = "已关联 " + origin.Metadata.Title
				if version := strings.TrimSpace(origin.Metadata.VersionName); version != "" {
					state.message += " · " + version
				}
			}
			if s.library.selectedID == state.instanceID {
				s.refreshAssets()
			}
		}
	})
}

func (s *Shell) closeAssetAssociation(state *assetAssociationState) {
	if s.market.association != state || state.active {
		return
	}
	s.market.association = nil
	s.market.showAssociation = false
	s.selectInstance(state.instanceID)
}

func (s *Shell) assetAssociationBanner(c *ui.Context, state *assetAssociationState) {
	t := s.theme
	ui.Column(c.Key("asset-association")).FillWidth().Shrink(0).Padding(6, 8).Gap(3).Radius(7).Background(t.Surface).Children(func() {
		ui.Row(c).FillWidth().Height(28).Gap(6).Children(func() {
			ui.Icon(c, iconScan).Size(16, 16).TextColor(colors(t).Icon)
			ui.Text(c, state.file.Name).FontSize(12).FontWeight(500).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
			if state.active {
				s.cancelAssetAssociationButton(c, state)
			} else {
				if state.origin == nil {
					ui.Button(c.Key("association-identify"), "自动识别").Height(24).
						Disabled(s.market.source == market.CurseForge && !s.market.actions.CurseForgeAvailable).
						OnClick(func() { s.startAssetAssociation(state, s.market.source, "", "") })
				}
				label := "返回实例"
				if state.origin != nil {
					label = "查看实例"
				}
				ui.Button(c.Key("association-return"), label).Height(24).OnClick(func() { s.closeAssetAssociation(state) })
			}
		})
		s.assetAssociationStatus(c, state)
	})
}

func (s *Shell) cancelAssetAssociationButton(c *ui.Context, state *assetAssociationState) {
	ui.Button(c.Key("association-cancel"), "取消").Height(24).Disabled(state.cancel == nil).OnClick(func() {
		if state.cancel != nil {
			state.cancel()
			state.cancel = nil
		}
	})
}

func (s *Shell) assetAssociationStatus(c *ui.Context, state *assetAssociationState) {
	text, color := state.message, s.theme.TextMuted
	if state.active {
		text = "正在核对本地文件…"
	} else if state.failed {
		color = s.theme.Danger
	}
	if text != "" {
		ui.Text(c, text).FontSize(11).TextColor(color).FillWidth().MaxLines(3)
	}
}

func (s *Shell) assetAssociationTask(c *ui.Context) {
	state := s.market.association
	if state == nil || (!state.active && state.message == "") {
		return
	}
	section(c, state.file.Name, func() {
		s.assetAssociationStatus(c, state)
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Button(c.Key("association-task-show"), "查看关联").Height(25).OnClick(func() { s.showAssetAssociation(state) })
			if state.active {
				s.cancelAssetAssociationButton(c, state)
			}
		})
	})
}

func (s *Shell) marketVersionAction(project market.Project, version market.Version, installDisabled bool) (string, bool, func()) {
	if state := s.marketAssociation(); state != nil {
		disabled := state.active || state.origin != nil || market.Kind(state.file.Kind) != project.Kind ||
			(project.Source == market.CurseForge && !s.market.actions.CurseForgeAvailable)
		return "核对来源", disabled, func() { s.startAssetAssociation(state, project.Source, project.ID, version.ID) }
	}
	return "安装", installDisabled, func() { s.selectMarketInstall(project, version) }
}
