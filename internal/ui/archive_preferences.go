package ui

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"mcl/internal/app"
)

func (s *Shell) initializeArchiveEditor(i *marketInstallState, plan app.ArchiveImportPlan) {
	if plan.Instance.ID == "" || plan.NeedsArchive {
		return
	}
	if !i.archiveEdited {
		i.archiveDescription = plan.Instance.Description
		i.archiveIconStyle = plan.Instance.IconStyle
		i.archiveIcon = slices.Clone(plan.Instance.Icon)
		if len(i.archiveIcon) > 0 {
			i.archiveBitmap, _ = ui.DecodeBitmap(i.archiveIcon)
		}
		i.archiveEdited = true
	}
	if i.archiveName == "" {
		i.archiveName = plan.Instance.Name
	}
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
	ui.Column(c.Key("archive-instance-preferences")).FillWidth().Shrink(0).Gap(8).Children(func() {
		ui.Row(c).FillWidth().Height(52).Gap(12).Children(func() {
			anchor := ui.ButtonBase(c.Key("archive-instance-icon")).Label("选择导入实例图标").Tooltip("更换图标").Size(44, 44).Radius(9).Center().Disabled(i.busy || i.archiveIconBusy)
			controlMotion(c, anchor, ui.Transparent, !i.busy && !i.archiveIconBusy).OnClick(func() { i.archiveIconOpen = !i.archiveIconOpen; s.gameDirectories.pickerOpen = false }).Children(func() { drawInstanceArtwork(c, style, i.archiveBitmap, 44) })
			creationPopover(c.Key("archive-icon-options"), anchor, &i.archiveIconOpen, 270, func() { s.archiveIconPicker(c, i) })
			ui.TextInput(c.Key("archive-edit-name"), &i.archiveName).Label("导入实例名称").Placeholder("实例名称").Height(28).Grow(1).MinWidth(0).Disabled(i.busy)
			if i.archiveIconBusy {
				busyIndicator(c, 12)
			}
		})
		ui.Column(c).FillWidth().Radius(10).Border(.5, colors(t).ListDivider).Clip().Children(func() {
			gameSettingRow(c, "description", "描述", func() {
				ui.TextInput(c.Key("archive-edit-description"), &i.archiveDescription).Label("导入实例描述").Placeholder("可选").Grow(1).MinWidth(0).Height(26).Disabled(i.busy)
			})
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
		ui.Column(c.Key("archive-package-metadata")).FillWidth().Padding(3, 0).Gap(3).Children(func() {
			label := archiveFormatLabel(plan.Manifest.Format)
			if plan.Manifest.Name != "" {
				label += " · " + plan.Manifest.Name
			}
			ui.Text(c, label).FontSize(12).MaxLines(2)
			parts := []string{}
			if plan.Manifest.Version != "" {
				parts = append(parts, plan.Manifest.Version)
			}
			if plan.Manifest.Author != "" {
				parts = append(parts, plan.Manifest.Author)
			}
			parts = append(parts, "Minecraft "+plan.Manifest.Minecraft)
			for _, required := range plan.Manifest.Loaders {
				parts = append(parts, marketLoaderLabel(required.Type)+" "+required.Version)
			}
			ui.Text(c, strings.Join(parts, " · ")).FontSize(11).TextColor(colors(t).Metadata).MaxLines(3)
		})
	})
}
func (s *Shell) archiveIconPicker(c *ui.Context, i *marketInstallState) {
	choose := func(style string) {
		i.archiveIconStyle = style
		i.archiveIcon = nil
		i.archiveBitmap = nil
		i.archiveIconOpen = false
	}
	ui.Column(c).FillWidth().Padding(6).Gap(8).Children(func() {
		ui.Button(c.Key("archive-icon-auto"), "自动").Height(25).FillWidth().OnClick(func() { choose("") })
		ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
			for _, style := range []string{"grass", "fabric", "forge", "neoforge", "quilt", "cat", "leaf", "spark", "craft"} {
				button := ui.ButtonBase(c.Key(style)).Label(instancePresetLabel(style)).Tooltip(instancePresetLabel(style)).Size(36, 36).Radius(7).Center()
				controlMotion(c, button, ui.Transparent, true).OnClick(func() { choose(style) }).Children(func() { drawInstanceArtwork(c, style, nil, 30) })
			}
		})
		ui.Button(c.Key("archive-icon-file"), "选择图片…").Height(26).FillWidth().Disabled(s.library.actions.PickIcon == nil || i.archiveIconBusy).OnClick(func() {
			i.archiveIconBusy, i.archiveIconOpen = true, false
			i.archiveIconCancel = s.library.actions.PickIcon(func(data []byte, err error) {
				if s.market.install != i {
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
