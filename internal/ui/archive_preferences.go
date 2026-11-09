package ui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"

	"mcl/internal/app"
	"mcl/internal/core/market"
)

func (s *Shell) initializeArchiveEditor(i *marketInstallState, plan app.ArchiveImportPlan) {
	if plan.Instance.ID == "" || plan.NeedsArchive {
		return
	}
	if !i.archiveEdited {
		i.archiveDescription = plan.Instance.Description
		if !i.archiveIconChosen && len(i.archiveIcon) == 0 && i.archiveBitmap == nil && i.archiveIconStyle == "" {
			i.archiveIconStyle = plan.Instance.IconStyle
			i.archiveIcon = slices.Clone(plan.Instance.Icon)
			if len(i.archiveIcon) > 0 {
				i.archiveBitmap, _ = ui.DecodeBitmap(i.archiveIcon)
			}
		}
		i.archiveEdited = true
	}
	if i.archiveName == "" {
		i.archiveName = plan.Instance.Name
	}
	s.loadArchiveProjectIcon(i)
}

func (s *Shell) cancelArchiveIcon(i *marketInstallState) {
	if i == nil {
		return
	}
	i.archiveIconGeneration++
	if i.archiveIconCancel != nil {
		i.archiveIconCancel()
	}
	i.archiveIconCancel = nil
	i.archiveIconBusy = false
}
func (s *Shell) loadArchiveProjectIcon(i *marketInstallState) {
	if i == nil || i.localArchive != nil || i.project.Kind != market.Modpack || i.archiveIconChosen || i.archiveIconBusy || len(i.archiveIcon) > 0 || i.archiveIconStyle != "" || s.market.actions.InstanceIcon == nil {
		return
	}
	address := i.project.IconURL
	if address == "" || address == i.archiveIconURL {
		return
	}
	i.archiveIconURL = address
	i.archiveIconGeneration++
	generation := i.archiveIconGeneration
	i.archiveIconBusy = true
	if cached := s.market.icons[address]; cached != nil {
		i.archiveBitmap = cached
	}
	i.archiveIconCancel = s.market.actions.InstanceIcon(address, func(data []byte, err error) {
		if s.market.install != i || generation != i.archiveIconGeneration || i.archiveIconChosen {
			return
		}
		i.archiveIconBusy = false
		i.archiveIconCancel = nil
		if err != nil {
			i.archiveBitmap = nil
			if !errors.Is(err, context.Canceled) {
				s.Notify(ToastWarning, "未能读取整合包图标")
			}
			return
		}
		bitmap, err := ui.DecodeBitmap(data)
		if err != nil {
			i.archiveBitmap = nil
			s.Notify(ToastWarning, "无法显示整合包图标")
			return
		}
		i.archiveIcon = slices.Clone(data)
		i.archiveBitmap = bitmap
		i.archiveIconStyle = ""
	})
}

// The project title is only an initial editable name. Remote catalog titles
// may exceed the instance limit now that no intermediate name form is shown.
func archiveInitialName(title string) string {
	title = strings.ToValidUTF8(title, "")
	title = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, title))
	runes := []rune(title)
	if len(runes) > 80 {
		title = strings.TrimSpace(string(runes[:80]))
	}
	if title == "" {
		return "整合包"
	}
	return title
}
func archiveFormatLabel(format string) string {
	switch format {
	case "modrinth":
		return "Modrinth"
	case "curseforge":
		return "CurseForge"
	case "multimc":
		return "MultiMC / Prism"
	default:
		return "整合包"
	}
}
func (s *Shell) archivePreferencesView(c *ui.Context, i *marketInstallState) {
	plan, t := i.archivePlan, s.theme
	if plan == nil {
		return
	}
	style := i.archiveIconStyle
	if style == "" {
		style = "grass"
		if required, ok := plan.Instance.RequiredLoader(); ok {
			style = required.Type
		}
	}
	parts := []string{archiveFormatLabel(plan.Manifest.Format)}
	if plan.Manifest.Version != "" {
		parts = append(parts, plan.Manifest.Version)
	}
	parts = append(parts, "Minecraft "+plan.Manifest.Minecraft)
	for _, required := range plan.Manifest.Loaders {
		parts = append(parts, marketLoaderLabel(required.Type)+" "+required.Version)
	}
	ui.Column(c.Key("archive-instance-preferences")).FillWidth().Shrink(0).Gap(12).Children(func() {
		ui.Row(c.Key("archive-identity")).FillWidth().Height(56).Gap(12).Children(func() {
			anchor := ui.ButtonBase(c.Key("archive-instance-icon")).Label("选择导入实例图标").Tooltip("更换图标").Size(48, 48).Shrink(0).Radius(10).Clip().Center().Disabled(i.busy)
			controlMotion(c, anchor, ui.Transparent, !i.busy).OnClick(func() { i.archiveIconOpen = !i.archiveIconOpen; s.gameDirectories.pickerOpen = false }).Children(func() { drawInstanceArtwork(c, style, i.archiveBitmap, 48) })
			creationPopover(c.Key("archive-icon-options"), anchor, &i.archiveIconOpen, 270, func() { s.archiveIconPicker(c, i) })
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.TextInputBase(c.Key("archive-edit-name"), &i.archiveName).Label("导入实例名称").Placeholder("实例名称").FontSize(15).FontWeight(500).Height(28).FillWidth().MinWidth(0).Disabled(i.busy)
				ui.Text(c, strings.Join(parts, " · ")).FontSize(11).TextColor(colors(t).Metadata).FillWidth().MaxLines(1).Ellipsis("…")
			})
			if i.archiveIconBusy {
				busyIndicator(c, 12)
			}
		})
		ui.Column(c.Key("archive-edit-fields")).FillWidth().Radius(10).Border(.5, colors(t).ListDivider).Clip().Children(func() {
			ui.TextAreaBase(c.Key("archive-edit-description"), &i.archiveDescription).Label("导入实例描述").Placeholder("添加描述…").FontSize(12).TextColor(t.TextMuted).FillWidth().Height(64).Shrink(0).Padding(10, 14).Disabled(i.busy)
			gameSettingsDivider(c)
			gameSettingRow(c, "directory", "游戏目录", func() {
				selected := plan.Instance.GameDirectoryID
				if plan.Request.GameDirectoryID != nil {
					selected = *plan.Request.GameDirectoryID
				}
				s.gameDirectoryMenu(c, "archive-confirm-directory", selected, i.busy, func(id string) {
					plan.Instance.GameDirectoryID = id
					plan.Request.GameDirectoryID = &id
					i.archiveDirectoryID = &id
				})
			})
		})
	})
}
func (s *Shell) archiveIconPicker(c *ui.Context, i *marketInstallState) {
	choose := func(style string) {
		s.cancelArchiveIcon(i)
		i.archiveIconChosen = style != ""
		i.archiveIconURL = ""
		i.archiveIconStyle = style
		i.archiveIcon = nil
		i.archiveBitmap = nil
		i.archiveIconOpen = false
		if style == "" {
			s.loadArchiveProjectIcon(i)
		}
	}
	ui.Column(c).FillWidth().Padding(6).Gap(8).Children(func() {
		ui.Button(c.Key("archive-icon-auto"), "自动").Height(25).FillWidth().OnClick(func() { choose("") })
		ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
			for _, style := range []string{"grass", "fabric", "forge", "neoforge", "quilt", "cat", "leaf", "spark", "craft"} {
				button := ui.ButtonBase(c.Key(style)).Label(instancePresetLabel(style)).Tooltip(instancePresetLabel(style)).Size(36, 36).Radius(7).Center()
				controlMotion(c, button, ui.Transparent, true).OnClick(func() { choose(style) }).Children(func() { drawInstanceArtwork(c, style, nil, 30) })
			}
		})
		ui.Button(c.Key("archive-icon-file"), "选择图片…").Height(26).FillWidth().Disabled(s.library.actions.PickIcon == nil).OnClick(func() {
			s.cancelArchiveIcon(i)
			i.archiveIconChosen = true
			generation := i.archiveIconGeneration
			i.archiveIconBusy, i.archiveIconOpen = true, false
			i.archiveIconCancel = s.library.actions.PickIcon(func(data []byte, err error) {
				if s.market.install != i || generation != i.archiveIconGeneration {
					return
				}
				i.archiveIconBusy, i.archiveIconCancel = false, nil
				if err != nil {
					if !errors.Is(err, context.Canceled) {
						s.Notify(ToastError, err.Error())
					}
					return
				}
				if len(data) == 0 {
					return
				}
				bitmap, err := ui.DecodeBitmap(data)
				if err != nil {
					s.Notify(ToastError, "无法读取实例图标")
					return
				}
				i.archiveIcon, i.archiveBitmap, i.archiveIconStyle = slices.Clone(data), bitmap, ""
			})
		})
	})
}
