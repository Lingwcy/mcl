package ui

import (
	"fmt"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/launch"

	"github.com/egoist/mygo/ui"
)

type GameActions struct{ Stop func(string, func(error)) }
type gameState struct {
	actions                   GameActions
	rows                      []app.GameSnapshot
	selected, filter, message string
	errorsOnly                bool
	list                      ui.ListState
}

func (s *Shell) ConnectGames(actions GameActions) {
	s.games.actions = actions
	s.games.list.FollowEnd = true
}
func (s *Shell) SetGames(rows []app.GameSnapshot) { s.games.rows = rows }

func (s *Shell) knownGame(id string) bool {
	for _, game := range s.games.rows {
		if game.ID == id {
			return true
		}
	}
	return false
}
func (s *Shell) instanceName(id, fallback string) string {
	for _, value := range s.library.snapshot.Instances {
		if value.ID == id {
			return value.Name
		}
	}
	return fallback
}

func gameStatus(value app.GameSnapshot) string {
	if value.Running {
		if value.Stopping {
			return "正在停止"
		}
		if value.Window == app.GameWindowPending {
			return "等待游戏窗口"
		}
		return "运行中"
	}
	if value.Exit.Stopped {
		return "已停止"
	}
	if value.Exit.Code == 0 {
		return "正常退出"
	}
	return fmt.Sprintf("异常退出 · 代码 %d", value.Exit.Code)
}

func (s *Shell) showLogs(id string) {
	s.games.selected, s.games.filter, s.games.message = id, "", ""
	s.games.list = ui.ListState{FollowEnd: true}
	s.page = logsPage
}

// Auto-open only once, at this launch's successful completion, while its own
// workspace is still visible. Snapshot refreshes never request navigation.
func (s *Shell) maybeOpenGameLogs(enabled, canceled bool, id string) {
	if !enabled || canceled || id == "" || s.page != launchPage || s.installation.launchGameID != id {
		return
	}
	s.showLogs(id)
}

func (s *Shell) stopGame(id string) {
	if s.games.actions.Stop == nil {
		return
	}
	s.games.message = ""
	s.games.actions.Stop(id, func(err error) {
		if err != nil {
			s.Notify(ToastError, err.Error())
		}
	})
}

func (s *Shell) gameCards(c *ui.Context) {
	for _, game := range s.games.rows {
		section(c, s.instanceName(game.InstanceID, game.VersionID), func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Textf(c, "%s · Minecraft %s", gameStatus(game), game.VersionID).Grow(1).FontSize(12)
				ui.Button(c.Key("logs-"+game.ID), "查看日志").Height(26).OnClick(func() { s.showLogs(game.ID) })
				if game.Running {
					ui.Button(c.Key("stop-"+game.ID), "停止游戏").Height(26).Disabled(game.Stopping).OnClick(func() { s.stopGame(game.ID) })
				}
			})
			ui.Textf(c, "%s · PID %d", game.StartedAt.Local().Format("15:04:05"), game.PID).FontSize(11).TextColor(colors(s.theme).Metadata)
			if game.Error != "" {
				ui.Text(c, game.Error).FontSize(11).TextColor(s.theme.Danger)
			}
		})
	}
}

func (s *Shell) logView(c *ui.Context) {
	g, t := &s.games, s.theme
	var lines []launch.Line
	title, status := "任务日志", "最近任务"
	var current *app.GameSnapshot
	if g.selected == "" {
		lines = s.installation.events
		title = s.installation.name + " · 任务日志"
	} else {
		for index := range g.rows {
			if g.rows[index].ID == g.selected {
				current = &g.rows[index]
				break
			}
		}
		if current != nil {
			lines = current.Output.Lines
			title = s.instanceName(current.InstanceID, current.VersionID)
			status = gameStatus(*current)
		} else {
			status = "此会话的日志已不在保留范围"
		}
	}
	query := strings.ToLower(strings.TrimSpace(g.filter))
	filtered := make([]launch.Line, 0, len(lines))
	for _, line := range lines {
		if g.errorsOnly && line.Stream != "stderr" {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(line.Text), query) {
			continue
		}
		filtered = append(filtered, line)
	}
	services := c.Services()
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(8, 28, 20).Gap(10).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			ui.Text(c, title).FontSize(18).Grow(1).MaxLines(1).Ellipsis("…")
			ui.Button(c.Key("back-tasks"), "返回任务").Height(26).OnClick(func() { s.page = activityPage })
		})
		ui.Row(c).Gap(10).Children(func() {
			ui.Text(c, status).FontSize(11).TextColor(t.TextMuted).Grow(1)
			if current != nil && current.Running {
				id := current.ID
				ui.Button(c.Key("log-stop"), "停止游戏").Height(24).Disabled(current.Stopping).OnClick(func() { s.stopGame(id) })
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			ui.TextInput(c.Key("log-filter"), &g.filter).Label("搜索日志").Placeholder("搜索日志").Grow(1).MinWidth(0).Height(26)
			label := "全部输出"
			if g.errorsOnly {
				label = "仅标准错误"
			}
			ui.Button(c.Key("log-errors"), label).Height(26).OnClick(func() { g.errorsOnly = !g.errorsOnly })
			ui.Button(c.Key("log-follow"), "回到最新").Height(26).OnClick(func() { g.list.ScrollToEnd() })
			ui.Button(c.Key("copy-logs"), "复制日志").Height(26).OnClick(func() {
				var text strings.Builder
				for _, line := range filtered {
					fmt.Fprintf(&text, "[%s] [%s] %s\n", line.Time.Local().Format("15:04:05"), line.Stream, line.Text)
				}
				services.WriteClipboard(text.String())
				s.Notify(ToastSuccess, "已复制当前筛选日志")
			})
		})
		if current != nil && (current.Output.Dropped > 0 || current.Exit.OutputIncomplete) {
			ui.Textf(c, "保留最近日志；已丢弃 %d 行。退出或脱离时可能未取得完整输出。", current.Output.Dropped).FontSize(11).TextColor(t.TextMuted)
		}
		if len(filtered) == 0 {
			ui.Text(c, "暂无匹配日志").FontSize(12).TextColor(t.TextMuted)
		}
		g.list.Key = func(index int) any { return filtered[index].Sequence }
		g.list.Label = func(index int) string { return filtered[index].Text }
		ui.List(c.Key("log-list-"+g.selected), &g.list, len(filtered), func(index int) {
			line := filtered[index]
			color := t.Text
			if line.Stream == "stderr" {
				color = t.Danger
			}
			ui.Column(c).FillWidth().Padding(5, 8).Gap(2).Children(func() {
				ui.Textf(c, "%s · %s", line.Time.Local().Format("15:04:05"), line.Stream).FontSize(10).TextColor(colors(t).Metadata)
				ui.Text(c, line.Text).FillWidth().FontSize(11).TextColor(color).MaxLines(8).Ellipsis("…").Selectable()
				if line.Truncated {
					ui.Text(c, "此行已截断").FontSize(10).TextColor(t.Warning)
				}
			})
		}).Grow(1).MinHeight(0).FillWidth().Radius(8).Background(t.Surface).Border(1, t.Border)
	})
}
