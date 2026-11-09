package ui

import (
	"image"
	"slices"
	"strings"

	"mcl/internal/app"
	"mcl/internal/core/download"
	"mcl/internal/core/install"
	"mcl/internal/core/market"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

type MarketActions struct {
	Search              func(market.Query, func(market.Results, error)) func()
	Categories          func(market.Source, market.Kind, func([]market.Category, error)) func()
	LoaderIcons         func(func([]market.LoaderIcon, error)) func()
	Detail              func(market.Project, string, string, func(app.MarketDetail, error)) func()
	VersionDetail       func(market.Source, string, string, func(market.Version, error)) func()
	Icon                func(string, func(image.Image, error)) func()
	SetKey              func(string) error
	CurseForgeAvailable bool
	Plan                func(market.InstallRequest, func(market.InstallPlan, error)) func()
	SupplyFiles         func(market.InstallPlan, func(app.LocalFileSupply, error)) func()
	Install             func(market.InstallPlan, func(download.Progress), func(error)) func()
	Worlds              func(string, func([]string, error)) func()
	InspectArchive      func(app.ArchiveImportRequest, app.Settings, func(download.Progress), func(app.ArchiveImportPlan, error)) func()
	SupplyArchive       func(app.ArchiveImportPlan, func(app.LocalFileSupply, error)) func()
	ImportArchive       func(app.ArchiveImportPlan, func(install.Progress), func(app.PackImportResult, error)) func()
	ResumePackImport    func(string, func(install.Progress), func(app.PackImportResult, error)) func()
	InspectPackRepair   func(string, func(app.PackRepairPlan, error)) func()
	SupplyPackRepair    func(app.PackRepairPlan, func(app.LocalFileSupply, error)) func()
	RestorePack         func(app.PackRepairPlan, []string, func(download.Progress), func(int, error)) func()
}
type marketState struct {
	importDirectoryID                                *string
	actions                                          MarketActions
	kind                                             app.AssetKind
	source                                           market.Source
	text, gameVersion, sort, targetID                string
	loaders, categoryIDs, environments               []string
	openSource                                       bool
	filterExpanded, filterMore                       map[string]bool
	page                                             int
	loaded, busy, detailBusy, categoriesBusy         bool
	error, categoryError, message                    string
	results                                          market.Results
	categories                                       []market.Category
	list, versionsList                               ui.ListState
	generation, detailGeneration, categoryGeneration uint64
	cancel, cancelDetail, cancelCategories           func()
	icons                                            map[string]*ui.Bitmap
	iconCancels                                      []func()
	selected                                         *market.Project
	detail                                           *app.MarketDetail
	descriptionTruncated                             bool
	tab                                              int
	markdown                                         markdown.Options
	keyInput, keyError                               string
	showKey                                          bool
	install                                          *marketInstallState
	showInstall                                      bool
	version                                          *marketVersionState
	association                                      *assetAssociationState
	showAssociation                                  bool
	loaderIcons                                      map[string]marketSymbol
	categoryIcons                                    map[string]marketSymbol
	categoryIconCancels                              []func()
	loadingLoaderIcons                               bool
}

func (s *Shell) ConnectMarket(actions MarketActions) {
	m := &s.market
	m.actions = actions
	m.source = market.Modrinth
	m.sort = "downloads"
	m.kind = app.AssetMod
	if s.page == marketPage {
		s.searchMarket(true)
		s.loadMarketCategories()
	}
}
func (s *Shell) marketQuery() market.Query {
	m := &s.market
	return market.Query{Source: m.source, Kind: market.Kind(m.kind), Text: strings.TrimSpace(m.text), GameVersion: m.gameVersion, Loaders: slices.Clone(m.loaders), Categories: slices.Clone(m.categoryIDs), Environments: slices.Clone(m.environments), OpenSource: m.openSource, Sort: m.sort, Page: m.page}
}
func (s *Shell) searchMarket(reset bool) {
	m := &s.market
	m.showInstall = false
	if m.actions.Search == nil {
		return
	}
	if reset {
		m.page = 0
	}
	if m.cancel != nil {
		m.cancel()
	}
	if m.cancelDetail != nil {
		m.cancelDetail()
		m.cancelDetail = nil
	}
	m.detailBusy = false
	s.closeMarketVersion()
	m.detailGeneration++
	for _, cancel := range m.iconCancels {
		cancel()
	}
	m.iconCancels = nil
	m.icons = make(map[string]*ui.Bitmap)
	m.generation++
	generation := m.generation
	m.busy, m.loaded, m.error, m.message = true, true, "", ""
	m.selected, m.detail = nil, nil
	m.list = ui.ListState{}
	query := s.marketQuery()
	m.cancel = m.actions.Search(query, func(results market.Results, err error) {
		if generation != m.generation {
			return
		}
		m.busy, m.cancel = false, nil
		if err != nil {
			m.error = err.Error()
			m.results = market.Results{}
			return
		}
		m.results = results
		for _, p := range results.Projects {
			if p.IconURL == "" || m.actions.Icon == nil {
				continue
			}
			address := p.IconURL
			cancel := m.actions.Icon(address, func(img image.Image, err error) {
				if generation == m.generation && err == nil && img != nil {
					m.icons[address] = ui.NewBitmap(img)
				}
			})
			m.iconCancels = append(m.iconCancels, cancel)
		}
	})
}
func (s *Shell) loadMarketCategories() {
	m := &s.market
	s.loadMarketLoaderIcons()
	if m.actions.Categories == nil {
		return
	}
	if m.cancelCategories != nil {
		m.cancelCategories()
	}
	m.categoryGeneration++
	for _, cancel := range m.categoryIconCancels {
		cancel()
	}
	m.categoryIconCancels = nil
	m.categoryIcons = make(map[string]marketSymbol)
	generation := m.categoryGeneration
	m.categoriesBusy, m.categoryError = true, ""
	m.categories = nil
	m.cancelCategories = m.actions.Categories(m.source, market.Kind(m.kind), func(categories []market.Category, err error) {
		if generation != m.categoryGeneration {
			return
		}
		m.categoriesBusy, m.cancelCategories = false, nil
		if err != nil {
			m.categoryError = err.Error()
			return
		}
		m.categories = categories
		s.loadCategoryIcons(categories, generation)
	})
}
func (s *Shell) switchMarketSource(source market.Source) {
	m := &s.market
	if m.source == source {
		m.showKey, m.showInstall, m.keyInput = false, false, ""
		s.backMarketResults()
		return
	}
	m.source = source
	m.resetFilters()
	m.showKey, m.showInstall, m.keyInput = false, false, ""
	m.sort = "downloads"
	if source == market.Modrinth && m.kind == app.AssetKind(market.World) {
		m.kind = app.AssetMod
	}
	s.loadMarketCategories()
	s.searchMarket(true)
}
func (s *Shell) openMarketProject(p market.Project) {
	m := &s.market
	if m.actions.Detail == nil {
		return
	}
	s.closeMarketVersion()
	if m.cancelDetail != nil {
		m.cancelDetail()
	}
	m.detailGeneration++
	generation := m.detailGeneration
	m.selected = &p
	m.detail = nil
	m.detailBusy = true
	m.error = ""
	m.message = ""
	m.tab = 0
	if s.marketAssociation() != nil {
		m.tab = 1
	}
	m.versionsList = ui.ListState{}
	loaders := slices.Clone(m.loaders)
	environments := slices.Clone(m.environments)
	m.cancelDetail = m.actions.Detail(p, m.gameVersion, m.singleLoader(), func(detail app.MarketDetail, err error) {
		if generation != m.detailGeneration {
			return
		}
		if len(loaders) > 1 || len(environments) > 0 {
			detail.Versions = slices.DeleteFunc(detail.Versions, func(version market.Version) bool {
				if len(loaders) > 1 {
					matched := false
					for _, loader := range loaders {
						matched = matched || slices.Contains(version.Loaders, loader)
					}
					if !matched {
						return true
					}
				}
				for _, side := range environments {
					if !market.MatchesEnvironment(version.Environment, side) {
						return true
					}
				}
				return false
			})
		}
		m.detailBusy, m.cancelDetail = false, nil
		if err != nil {
			m.error = err.Error()
			return
		}
		detail.Project.Body, m.descriptionTruncated = marketDisplayText(detail.Project.Body)
		m.detail = &detail
		if address := detail.Project.IconURL; address != "" && m.icons[address] == nil && m.actions.Icon != nil {
			cancel := m.actions.Icon(address, func(img image.Image, err error) {
				if generation != m.detailGeneration || err != nil || img == nil {
					return
				}
				if m.icons == nil {
					m.icons = make(map[string]*ui.Bitmap)
				}
				m.icons[address] = ui.NewBitmap(img)
			})
			m.iconCancels = append(m.iconCancels, cancel)
		}
	})
}

func (m *marketState) resetFilters() {
	m.loaders, m.categoryIDs, m.environments = nil, nil, nil
	m.openSource = false
}

func (m *marketState) filtersActive() bool {
	return len(m.loaders) > 0 || len(m.categoryIDs) > 0 || len(m.environments) > 0 || m.openSource
}

func (m *marketState) singleLoader() string {
	if len(m.loaders) == 1 {
		return m.loaders[0]
	}
	return ""
}

func (s *Shell) selectMarketGame(game string) {
	m := &s.market
	m.gameVersion = game
	if game == "" && m.source == market.CurseForge {
		m.loaders = nil
	}
	s.searchMarket(true)
}
func (s *Shell) backMarketResults() {
	m := &s.market
	s.closeMarketVersion()
	if m.cancelDetail != nil {
		m.cancelDetail()
	}
	m.detailGeneration++
	m.selected, m.detail = nil, nil
	m.error, m.detailBusy, m.showInstall = "", false, false
}
func (s *Shell) selectMarketTarget(id string) {
	m := &s.market
	m.targetID = id
	if id != "" {
		for _, value := range s.library.snapshot.Instances {
			if value.ID == id {
				m.gameVersion = value.Version.ID
				break
			}
		}
	}
	s.searchMarket(true)
}
func (s *Shell) openMarketKey() {
	m := &s.market
	m.keyInput, m.keyError, m.showKey = "", "", true
}

func (s *Shell) applyMarketKey(key string) {
	m := &s.market
	if m.actions.SetKey == nil {
		return
	}
	if err := m.actions.SetKey(key); err != nil {
		m.keyError = err.Error()
		return
	}
	m.actions.CurseForgeAvailable = key != ""
	m.keyInput, m.keyError = "", ""
	m.showKey = false
	s.loadMarketCategories()
	s.searchMarket(true)
}
