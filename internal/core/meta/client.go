package meta

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mcl/internal/core/diagnostics"
	"mcl/internal/fileio"
)

const ManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

const maxMetadataSize = 16 << 20

type Catalog struct {
	Manifest Manifest
	Cached   bool
	Stale    bool
	Warning  error
}

// Client caches mutable manifests by age and immutable version documents by
// their official SHA-1. HTTP is injectable so ordinary tests never need a network.
type Client struct {
	HTTP          *http.Client
	CacheDir      string
	ManifestURL   string
	MaxAge        time.Duration
	VerifiedFetch func(context.Context, string, string, int64) ([]byte, error)
	manifestMu    sync.Mutex
}

func NewClient(cacheDir string) *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 25 * time.Second}, CacheDir: cacheDir,
		ManifestURL: ManifestURL, MaxAge: time.Hour,
	}
}

func (c *Client) Manifest(ctx context.Context, refresh bool) (Catalog, error) {
	c.manifestMu.Lock()
	defer c.manifestMu.Unlock()
	if err := ctx.Err(); err != nil {
		return Catalog{}, err
	}
	cachePath := filepath.Join(c.CacheDir, "manifest.json")
	data, cacheErr := fileio.Read(ctx, cachePath, maxMetadataSize)
	var cached Manifest
	if cacheErr == nil {
		cached, cacheErr = ParseManifest(data)
	}
	if !refresh && cacheErr == nil {
		if stat, err := os.Stat(cachePath); err == nil && time.Since(stat.ModTime()) <= c.MaxAge {
			return Catalog{Manifest: cached, Cached: true}, nil
		}
	}
	data, err := c.fetch(ctx, c.ManifestURL)
	if err != nil {
		if ctx.Err() != nil {
			return Catalog{}, ctx.Err()
		}
		if cacheErr == nil {
			return Catalog{Manifest: cached, Cached: true, Stale: true, Warning: err}, nil
		}
		return Catalog{}, fmt.Errorf("获取版本列表失败，且没有可用缓存: %w", err)
	}
	manifest, err := ParseManifest(data)
	if err != nil {
		return Catalog{}, err
	}
	cacheErr = fileio.Write(ctx, cachePath, data, 0o600)
	if ctx.Err() != nil {
		return Catalog{}, ctx.Err()
	}
	return Catalog{Manifest: manifest, Warning: cacheErr}, nil
}

func (c *Client) versionDocument(ctx context.Context, entry ManifestEntry) (Version, error) {
	if err := ctx.Err(); err != nil {
		return Version{}, err
	}
	if err := entry.Validate(); err != nil {
		return Version{}, err
	}
	directory := filepath.Join(c.CacheDir, "versions")
	if officialPackage(entry) {
		// Do not promote same-hash documents previously fetched from an arbitrary
		// endpoint into official provenance. Old/general cache entries stay intact.
		directory = filepath.Join(directory, "mojang-v1")
	}
	cachePath := filepath.Join(directory, strings.ToLower(entry.SHA1)+".json")
	parse := func(data []byte) (Version, error) {
		digest := sha1.Sum(data)
		if !strings.EqualFold(hex.EncodeToString(digest[:]), entry.SHA1) {
			return Version{}, fmt.Errorf("version %s metadata checksum mismatch", entry.ID)
		}
		version, err := ParseVersion(data)
		if err != nil {
			return Version{}, err
		}
		if version.ID != entry.ID {
			return Version{}, errors.New("version ID does not match its manifest entry")
		}
		return version, nil
	}
	if data, err := fileio.Read(ctx, cachePath, maxMetadataSize); err == nil {
		if version, err := parse(data); err == nil {
			return version, nil
		}
	}
	var data []byte
	var err error
	if c.VerifiedFetch != nil {
		data, err = c.VerifiedFetch(ctx, entry.URL, entry.SHA1, maxMetadataSize)
	} else {
		data, err = c.fetch(ctx, entry.URL)
	}
	if err != nil {
		return Version{}, err
	}
	version, err := parse(data)
	if err != nil {
		return Version{}, err
	}
	if err := fileio.Write(ctx, cachePath, data, 0o600); err != nil {
		return Version{}, fmt.Errorf("cache version metadata: %w", err)
	}
	return version, nil
}

func (c *Client) fetch(ctx context.Context, address string) ([]byte, error) {
	ctx, evidence := diagnostics.Start(ctx, "metadata", "minecraft-metadata")
	defer evidence.End()
	evidence.Network(diagnostics.Source(address), 0, 1)
	if err := ValidateURL(address); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "mcl/0.1.0")
	response, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch metadata: %w", err)
	}
	defer response.Body.Close()
	evidence.Network(diagnostics.Source(address), response.StatusCode, 1)
	if response.StatusCode != http.StatusOK {
		return nil, diagnostics.Wrap(diagnostics.HTTPCode(response.StatusCode), fmt.Errorf("metadata service returned HTTP %d", response.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxMetadataSize+1))
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	if len(data) > maxMetadataSize {
		return nil, errors.New("metadata response exceeds 16 MiB")
	}
	evidence.Work()
	return data, nil
}
