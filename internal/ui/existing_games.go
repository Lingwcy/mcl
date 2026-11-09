package ui

import (
	"context"
	"errors"
	"slices"

	"github.com/egoist/mygo/ui"
	"mcl/internal/app"
	"mcl/internal/core/discovery"
)

type ExistingGamesActions struct {
	List   func(func([]app.ExistingSource, error)) func()
	Add    func(func(app.ExistingSource, error)) func()
	Remove func(string, func(error)) func()
	Scan   func(string, func(discovery.Report, error)) func()
}
type existingGamesState struct {
	actions                     ExistingGamesActions
	sources                     []app.ExistingSource
	sourceID, selectedID, query string
	report                      discovery.Report
	list                        ui.ListState
	busy, scanned, closed       bool
	operation                   string
	cancel                      func()
	generation                  uint64
	returnPage                  page
}

func (s *Shell) ConnectExistingGames(actions ExistingGamesActions) { s.existing.actions = actions }
func (s *Shell) openExistingGames() {
	d := &s.existing
	if s.page != existingGamesPage {
		d.returnPage = s.page
	}
	s.page = existingGamesPage
	d.selectedID = ""
	s.loadExistingSources(true)
}
func (s *Shell) cancelExistingGames() {
	d := &s.existing
	d.generation++
	if d.cancel != nil {
		d.cancel()
	}
	d.cancel = nil
	d.busy = false
	d.operation = ""
}
func (s *Shell) closeExistingGames() {
	s.cancelExistingGames()
	destination := s.existing.returnPage
	if destination == "" || destination == existingGamesPage {
		destination = newInstancePage
	}
	s.page = destination
}
func (s *Shell) existingTask(operation string, run func(func(error)) func(), success func()) {
	s.cancelExistingGames()
	d := &s.existing
	d.busy = true
	d.operation = operation
	generation := d.generation
	d.cancel = run(func(err error) {
		if d.closed || generation != d.generation || s.page != existingGamesPage {
			return
		}
		d.busy = false
		d.cancel = nil
		d.operation = ""
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				s.Notify(ToastError, err.Error())
			}
			return
		}
		if success != nil {
			success()
		}
	})
}
func (s *Shell) loadExistingSources(scan bool) {
	d := &s.existing
	if d.actions.List == nil || d.busy {
		return
	}
	var result []app.ExistingSource
	s.existingTask("list", func(done func(error)) func() {
		return d.actions.List(func(rows []app.ExistingSource, err error) {
			result = slices.Clone(rows)
			done(err)
		})
	}, func() {
		d.sources = result
		if !slices.ContainsFunc(d.sources, func(value app.ExistingSource) bool { return value.ID == d.sourceID }) {
			d.sourceID = ""
			d.report = discovery.Report{}
			d.scanned = false
			if len(d.sources) > 0 {
				d.sourceID = d.sources[0].ID
			}
		}
		if scan && d.sourceID != "" {
			s.scanExistingGames(d.sourceID)
		}
	})
}
func (s *Shell) addExistingSource() {
	d := &s.existing
	if d.busy || d.actions.Add == nil {
		return
	}
	var added app.ExistingSource
	s.existingTask("add", func(done func(error)) func() {
		return d.actions.Add(func(value app.ExistingSource, err error) { added = value; done(err) })
	}, func() {
		d.sources = append(d.sources, added)
		s.scanExistingGames(added.ID)
	})
}
func (s *Shell) removeExistingSource() {
	d := &s.existing
	if d.busy || d.sourceID == "" || d.actions.Remove == nil {
		return
	}
	id := d.sourceID
	s.existingTask("remove", func(done func(error)) func() { return d.actions.Remove(id, done) }, func() {
		d.sources = slices.DeleteFunc(d.sources, func(source app.ExistingSource) bool { return source.ID == id })
		d.sourceID, d.selectedID = "", ""
		d.scanned = false
		d.report = discovery.Report{}
		s.Notify(ToastInfo, "已移除只读来源登记，原游戏文件未改动")
	})
}
func (s *Shell) scanExistingGames(id string) {
	d := &s.existing
	if d.actions.Scan == nil {
		return
	}
	s.cancelExistingGames()
	d.sourceID, d.selectedID = id, ""
	d.report = discovery.Report{}
	d.scanned = false
	var result discovery.Report
	s.existingTask("scan", func(done func(error)) func() {
		return d.actions.Scan(id, func(report discovery.Report, err error) { result = report; done(err) })
	}, func() {
		d.report = result
		d.scanned = true
	})
}
func (s *Shell) existingSource() (app.ExistingSource, bool) {
	for _, source := range s.existing.sources {
		if source.ID == s.existing.sourceID {
			return source, true
		}
	}
	return app.ExistingSource{}, false
}
