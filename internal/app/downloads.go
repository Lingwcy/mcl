package app

import (
	"context"
	"mcl/internal/core/download"
	"net/http"
)

type DownloadCacheStore interface {
	Inspect(context.Context) (download.CacheInfo, error)
	Clear(context.Context, string) (download.CacheInfo, error)
}
type DownloadService struct {
	cache     DownloadCacheStore
	transport func() http.RoundTripper
}

func NewDownloadService(cache DownloadCacheStore, transport func() http.RoundTripper) *DownloadService {
	return &DownloadService{cache: cache, transport: transport}
}
func (s *DownloadService) CacheInfo(ctx context.Context) (download.CacheInfo, error) {
	return s.cache.Inspect(ctx)
}
func (s *DownloadService) ClearCache(ctx context.Context, expected string) (download.CacheInfo, error) {
	return s.cache.Clear(ctx, expected)
}
func (s *DownloadService) Check(ctx context.Context, source download.SourceMode) ([]download.ConnectivityResult, error) {
	return download.CheckConnectivity(ctx, &http.Client{Transport: s.transport()}, source)
}
