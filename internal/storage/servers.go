package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"
	"unicode/utf8"

	"mcl/internal/app"
	"mcl/internal/core/instance"
	"mcl/internal/core/server"
	"mcl/internal/fileio"
	"mcl/internal/platform/instancelock"
)

const (
	serversSchemaVersion = 1
	maxServers           = 512
	maxDismissedServers  = 2048
	maxServersFileBytes  = 4 << 20
	maxServerIconBytes   = 128 << 10
)

type serversDocument struct {
	Version       int                `json:"version"`
	Servers       []app.ServerRecord `json:"servers,omitempty"`
	DismissedKeys []string           `json:"dismissed_keys,omitempty"`
}

// ServerStore persists the launcher's global server list and merges servers
// discovered from managed instance game directories without mutating instances.
type ServerStore struct{ root string }

func NewServerStore(root string) *ServerStore { return &ServerStore{root: root} }

func (s *ServerStore) lock(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lock, err := instancelock.AcquireServers(s.root)
	if errors.Is(err, instancelock.ErrBusy) {
		return nil, errors.New("另一个 mcl 窗口正在更新服务器列表，请稍后重试")
	}
	if err != nil {
		return nil, err
	}
	return func() { _ = lock.Close() }, nil
}

func (s *ServerStore) read(ctx context.Context) (serversDocument, error) {
	data, err := fileio.Read(ctx, filepath.Join(s.root, "servers.json"), maxServersFileBytes)
	if errors.Is(err, os.ErrNotExist) {
		return serversDocument{Version: serversSchemaVersion}, nil
	}
	if err != nil {
		return serversDocument{}, err
	}
	var doc serversDocument
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(&doc) != nil || !errors.Is(dec.Decode(new(any)), io.EOF) || doc.Version != serversSchemaVersion {
		return serversDocument{}, errors.New("无法读取服务器列表配置，已保留原文件")
	}
	if err := validateServersDocument(doc); err != nil {
		return serversDocument{}, err
	}
	return doc, nil
}

func (s *ServerStore) write(ctx context.Context, doc serversDocument) error {
	doc.Version = serversSchemaVersion
	if err := validateServersDocument(doc); err != nil {
		return err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return fileio.Write(ctx, filepath.Join(s.root, "servers.json"), append(data, '\n'), 0o600)
}

func validateServersDocument(doc serversDocument) error {
	if doc.Version != serversSchemaVersion {
		return errors.New("服务器列表配置版本不受支持")
	}
	if len(doc.Servers) > maxServers || len(doc.DismissedKeys) > maxDismissedServers {
		return errors.New("服务器列表数量超出限制")
	}
	seen := make(map[string]bool, len(doc.Servers))
	for _, rec := range doc.Servers {
		if !instance.ValidID(rec.ID) {
			return errors.New("服务器列表包含无效条目 ID")
		}
		key, _, _, _, err := server.NormalizeAddress(rec.Address)
		if err != nil || rec.Key != key || seen[rec.Key] {
			return errors.New("服务器列表包含无效或重复地址")
		}
		seen[rec.Key] = true
		if !utf8.ValidString(rec.Name) || utf8.RuneCountInString(rec.Name) > 120 {
			return errors.New("服务器名称格式无效")
		}
		if rec.Source != "manual" && rec.Source != "instance" {
			return errors.New("服务器来源标记无效")
		}
		if rec.InstanceID != "" && !instance.ValidID(rec.InstanceID) {
			return errors.New("服务器关联的实例 ID 无效")
		}
		for _, id := range rec.InstanceIDs {
			if !instance.ValidID(id) {
				return errors.New("服务器来源实例列表无效")
			}
		}
		if len(rec.Icon) > maxServerIconBytes {
			return errors.New("服务器图标大小超出限制")
		}
		if len(rec.Icon) > 0 && (len(rec.Icon) < 8 || string(rec.Icon[:8]) != "\x89PNG\r\n\x1a\n") {
			return errors.New("服务器图标格式无效")
		}
	}
	return nil
}

func (s *ServerStore) Load(ctx context.Context) ([]app.ServerRecord, error) {
	doc, err := s.read(ctx)
	if err != nil {
		return nil, err
	}
	return cloneServerRecords(doc.Servers), nil
}

type discoveredAggregate struct {
	entry       server.DiscoveredServer
	instanceIDs []string
}

func (s *ServerStore) SyncInstances(ctx context.Context, includeDismissed bool) ([]app.ServerRecord, error) {
	instancesDir := filepath.Join(s.root, "instances")
	instances, err := NewInstanceStore(instancesDir).List(ctx)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	assets := NewAssetStore(instancesDir)
	var discoveredOrder []string
	discoveredMap := make(map[string]*discoveredAggregate)
	for _, inst := range instances {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries := scanInstanceServers(ctx, assets, inst.ID)
		for _, entry := range entries {
			agg, ok := discoveredMap[entry.Key]
			if !ok {
				agg = &discoveredAggregate{entry: entry}
				discoveredMap[entry.Key] = agg
				discoveredOrder = append(discoveredOrder, entry.Key)
			} else {
				if agg.entry.Name == "" && entry.Name != "" {
					agg.entry.Name = entry.Name
				}
				if len(agg.entry.Icon) == 0 && len(entry.Icon) > 0 {
					agg.entry.Icon = entry.Icon
				}
			}
			if !slices.Contains(agg.instanceIDs, inst.ID) {
				agg.instanceIDs = append(agg.instanceIDs, inst.ID)
			}
		}
	}

	unlock, err := s.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()

	doc, err := s.read(ctx)
	if err != nil {
		return nil, err
	}
	changed := false
	if includeDismissed && len(doc.DismissedKeys) > 0 {
		doc.DismissedKeys = nil
		changed = true
	}

	dismissed := make(map[string]bool, len(doc.DismissedKeys))
	for _, key := range doc.DismissedKeys {
		dismissed[key] = true
	}
	existing := make(map[string]int, len(doc.Servers))
	for i, rec := range doc.Servers {
		existing[rec.Key] = i
	}

	now := time.Now().UTC()
	for _, key := range discoveredOrder {
		agg := discoveredMap[key]
		if idx, ok := existing[key]; ok {
			rec := &doc.Servers[idx]
			for _, id := range agg.instanceIDs {
				if !slices.Contains(rec.InstanceIDs, id) {
					rec.InstanceIDs = append(rec.InstanceIDs, id)
					changed = true
				}
			}
			if rec.InstanceID == "" && len(agg.instanceIDs) > 0 {
				rec.InstanceID = agg.instanceIDs[0]
				changed = true
			}
			if len(rec.Icon) == 0 && len(agg.entry.Icon) > 0 {
				rec.Icon = slices.Clone(agg.entry.Icon)
				changed = true
			}
			continue
		}
		if dismissed[key] || len(doc.Servers) >= maxServers {
			continue
		}
		name := agg.entry.Name
		if name == "" {
			name = agg.entry.Address
		}
		preferred := ""
		if len(agg.instanceIDs) > 0 {
			preferred = agg.instanceIDs[0]
		}
		existing[key] = len(doc.Servers)
		doc.Servers = append(doc.Servers, app.ServerRecord{
			ID:          instance.NewID(),
			Name:        name,
			Address:     agg.entry.Address,
			Key:         key,
			Source:      "instance",
			InstanceID:  preferred,
			InstanceIDs: slices.Clone(agg.instanceIDs),
			Icon:        slices.Clone(agg.entry.Icon),
			AddedAt:     now,
		})
		changed = true
	}

	if changed {
		if err := s.write(ctx, doc); err != nil {
			return nil, err
		}
	}
	return cloneServerRecords(doc.Servers), nil
}

func scanInstanceServers(ctx context.Context, assets *AssetStore, instanceID string) []server.DiscoveredServer {
	game, err := assets.OpenGame(ctx, instanceID)
	if err != nil {
		return nil
	}
	defer game.Close()
	info, err := game.Lstat("servers.dat")
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	file, err := game.Open("servers.dat")
	if err != nil {
		return nil
	}
	defer file.Close()
	entries, err := server.ReadServersDat(file)
	if err != nil {
		return nil
	}
	return entries
}

func (s *ServerStore) Add(ctx context.Context, req app.AddServerRequest) (app.ServerRecord, []app.ServerRecord, error) {
	key, display, _, _, err := server.NormalizeAddress(req.Address)
	if err != nil {
		return app.ServerRecord{}, nil, err
	}
	if req.InstanceID != "" && !instance.ValidID(req.InstanceID) {
		return app.ServerRecord{}, nil, errors.New("关联的实例 ID 无效")
	}
	name := server.SanitizeName(req.Name)

	unlock, err := s.lock(ctx)
	if err != nil {
		return app.ServerRecord{}, nil, err
	}
	defer unlock()

	doc, err := s.read(ctx)
	if err != nil {
		return app.ServerRecord{}, nil, err
	}
	doc.DismissedKeys = slices.DeleteFunc(doc.DismissedKeys, func(v string) bool { return v == key })

	var saved app.ServerRecord
	if idx := slices.IndexFunc(doc.Servers, func(r app.ServerRecord) bool { return r.Key == key }); idx >= 0 {
		rec := &doc.Servers[idx]
		if name != "" {
			rec.Name = name
		} else if rec.Name == "" {
			rec.Name = display
		}
		rec.Address = display
		rec.Source = "manual"
		if req.InstanceID != "" {
			rec.InstanceID = req.InstanceID
		}
		saved = *rec
	} else {
		if len(doc.Servers) >= maxServers {
			return app.ServerRecord{}, nil, errors.New("服务器数量已达上限")
		}
		if name == "" {
			name = display
		}
		saved = app.ServerRecord{
			ID:         instance.NewID(),
			Name:       name,
			Address:    display,
			Key:        key,
			Source:     "manual",
			InstanceID: req.InstanceID,
			AddedAt:    time.Now().UTC(),
		}
		doc.Servers = append([]app.ServerRecord{saved}, doc.Servers...)
	}

	if err := s.write(ctx, doc); err != nil {
		return app.ServerRecord{}, nil, err
	}
	return saved, cloneServerRecords(doc.Servers), nil
}

func (s *ServerStore) SetInstance(ctx context.Context, key, instanceID string) ([]app.ServerRecord, error) {
	if instanceID != "" && !instance.ValidID(instanceID) {
		return nil, errors.New("关联的实例 ID 无效")
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()

	doc, err := s.read(ctx)
	if err != nil {
		return nil, err
	}
	idx := slices.IndexFunc(doc.Servers, func(r app.ServerRecord) bool { return r.Key == key })
	if idx < 0 {
		return cloneServerRecords(doc.Servers), nil
	}
	if doc.Servers[idx].InstanceID != instanceID {
		doc.Servers[idx].InstanceID = instanceID
		if err := s.write(ctx, doc); err != nil {
			return nil, err
		}
	}
	return cloneServerRecords(doc.Servers), nil
}

func (s *ServerStore) CacheStatus(ctx context.Context, key string, status server.Status) error {
	if !status.Online || key == "" {
		return nil
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	doc, err := s.read(ctx)
	if err != nil {
		return err
	}
	idx := slices.IndexFunc(doc.Servers, func(r app.ServerRecord) bool { return r.Key == key })
	if idx < 0 {
		return nil
	}
	rec := &doc.Servers[idx]
	changed := false
	if len(status.Favicon) > 0 && len(status.Favicon) <= maxServerIconBytes && !bytes.Equal(rec.Icon, status.Favicon) {
		rec.Icon = slices.Clone(status.Favicon)
		changed = true
	}
	if len(status.MOTD) > 0 && !motdEqual(rec.MOTD, status.MOTD) {
		rec.MOTD = cloneMOTD(status.MOTD)
		changed = true
	}
	if status.PlainMOTD != "" && rec.PlainMOTD != status.PlainMOTD {
		rec.PlainMOTD = status.PlainMOTD
		changed = true
	}
	if status.Version != "" && rec.Version != status.Version {
		rec.Version = status.Version
		changed = true
	}
	if !changed {
		return nil
	}
	return s.write(ctx, doc)
}

func (s *ServerStore) Remove(ctx context.Context, key string) ([]app.ServerRecord, error) {
	unlock, err := s.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()

	doc, err := s.read(ctx)
	if err != nil {
		return nil, err
	}
	before := len(doc.Servers)
	doc.Servers = slices.DeleteFunc(doc.Servers, func(r app.ServerRecord) bool { return r.Key == key })
	if len(doc.Servers) == before {
		return cloneServerRecords(doc.Servers), nil
	}
	if key != "" && !slices.Contains(doc.DismissedKeys, key) {
		if len(doc.DismissedKeys) >= maxDismissedServers {
			doc.DismissedKeys = doc.DismissedKeys[1:]
		}
		doc.DismissedKeys = append(doc.DismissedKeys, key)
	}
	if err := s.write(ctx, doc); err != nil {
		return nil, err
	}
	return cloneServerRecords(doc.Servers), nil
}

func cloneServerRecords(src []app.ServerRecord) []app.ServerRecord {
	out := make([]app.ServerRecord, len(src))
	for i, rec := range src {
		rec.InstanceIDs = slices.Clone(rec.InstanceIDs)
		rec.Icon = slices.Clone(rec.Icon)
		rec.MOTD = cloneMOTD(rec.MOTD)
		out[i] = rec
	}
	return out
}

func cloneMOTD(src []server.MOTDLine) []server.MOTDLine {
	if len(src) == 0 {
		return nil
	}
	out := make([]server.MOTDLine, len(src))
	for i, line := range src {
		out[i] = server.MOTDLine{Spans: slices.Clone(line.Spans)}
	}
	return out
}

func motdEqual(a, b []server.MOTDLine) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !slices.Equal(a[i].Spans, b[i].Spans) {
			return false
		}
	}
	return true
}
