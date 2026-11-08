package app

import (
	"context"
	"errors"
	"slices"
	"sync"

	"mcl/internal/core/market"
)

// MarketService captures the source client for each operation. Replacing the
// session-only API key cannot mutate a client used by an in-flight request.
type MarketService struct {
	mu     sync.RWMutex
	client *market.Client
}

func NewMarketService(client *market.Client) *MarketService { return &MarketService{client: client} }
func (s *MarketService) Client() *market.Client             { s.mu.RLock(); defer s.mu.RUnlock(); return s.client }
func (s *MarketService) SetCurseForgeKey(key string) error {
	if len(key) > 500 {
		return errors.New("API Key 过长")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	replacement := market.NewClient(key)
	if s.client != nil {
		replacement.HTTP = s.client.HTTP
		replacement.ModrinthURL, replacement.CurseForgeURL = s.client.ModrinthURL, s.client.CurseForgeURL
	}
	s.client = replacement
	return nil
}
func (s *MarketService) Search(ctx context.Context, q market.Query) (market.Results, error) {
	return s.Client().Search(ctx, q)
}
func (s *MarketService) Categories(ctx context.Context, source market.Source, kind market.Kind) ([]market.Category, error) {
	return s.Client().Categories(ctx, source, kind)
}

func (s *MarketService) VersionDetail(ctx context.Context, source market.Source, project, version string) (market.Version, error) {
	return s.Client().VersionDetail(ctx, source, project, version)
}

type MarketDetail struct {
	Project  market.Project
	Versions []market.Version
}

func (s *MarketService) Detail(ctx context.Context, p market.Project, game, loader string) (MarketDetail, error) {
	client := s.Client()
	project, err := client.Project(ctx, p.Source, p.ID, p.Kind)
	if err != nil {
		return MarketDetail{}, err
	}
	if project.ID != p.ID {
		return MarketDetail{}, errors.New("市场项目身份不匹配")
	}
	if project.Author == "" {
		project.Author = p.Author
	}
	if project.IconURL == "" {
		project.IconURL = p.IconURL
	}
	versions, err := client.Versions(ctx, p.Source, p.ID, game, loader)
	if err != nil {
		return MarketDetail{}, err
	}
	// Enforce filters locally as well, before exposing an installable choice.
	versions = slices.DeleteFunc(versions, func(v market.Version) bool {
		return (game != "" && !slices.Contains(v.GameVersions, game)) || (loader != "" && !slices.Contains(v.Loaders, loader))
	})
	return MarketDetail{Project: project, Versions: versions}, nil
}
