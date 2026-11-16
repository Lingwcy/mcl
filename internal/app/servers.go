package app

import (
	"context"
	"errors"
	"time"

	"mcl/internal/core/server"
)

// ServerRecord represents a deduplicated multiplayer server maintained by the
// launcher in its own global store.
type ServerRecord struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Address     string            `json:"address"`
	Key         string            `json:"key"`
	Source      string            `json:"source"`
	InstanceID  string            `json:"instance_id,omitempty"`
	InstanceIDs []string          `json:"instance_ids,omitempty"`
	Icon        []byte            `json:"icon,omitempty"`
	MOTD        []server.MOTDLine `json:"motd,omitempty"`
	PlainMOTD   string            `json:"plain_motd,omitempty"`
	Version     string            `json:"version,omitempty"`
	AddedAt     time.Time         `json:"added_at"`
}

// AddServerRequest defines the inputs for adding a server globally.
type AddServerRequest struct {
	Name       string
	Address    string
	InstanceID string
}

// ServerStore persists and merges global server records.
type ServerStore interface {
	Load(ctx context.Context) ([]ServerRecord, error)
	SyncInstances(ctx context.Context, includeDismissed bool) ([]ServerRecord, error)
	Add(ctx context.Context, request AddServerRequest) (ServerRecord, []ServerRecord, error)
	SetInstance(ctx context.Context, key, instanceID string) ([]ServerRecord, error)
	CacheStatus(ctx context.Context, key string, status server.Status) error
	Remove(ctx context.Context, key string) ([]ServerRecord, error)
}

// ServerService coordinates scanning instance server lists, global server
// management, and Server List Ping queries.
type ServerService struct {
	store ServerStore
}

func NewServerService(store ServerStore) *ServerService {
	return &ServerService{store: store}
}

func (s *ServerService) LoadAndSync(ctx context.Context, forceRescan bool) ([]ServerRecord, error) {
	if s.store == nil {
		return nil, errors.New("服务器存储未配置")
	}
	return s.store.SyncInstances(ctx, forceRescan)
}

func (s *ServerService) Add(ctx context.Context, request AddServerRequest) (ServerRecord, []ServerRecord, error) {
	if s.store == nil {
		return ServerRecord{}, nil, errors.New("服务器存储未配置")
	}
	return s.store.Add(ctx, request)
}

func (s *ServerService) SetInstance(ctx context.Context, key, instanceID string) ([]ServerRecord, error) {
	if s.store == nil {
		return nil, errors.New("服务器存储未配置")
	}
	return s.store.SetInstance(ctx, key, instanceID)
}

func (s *ServerService) Remove(ctx context.Context, key string) ([]ServerRecord, error) {
	if s.store == nil {
		return nil, errors.New("服务器存储未配置")
	}
	return s.store.Remove(ctx, key)
}

func (s *ServerService) Ping(ctx context.Context, key, address string) (server.Status, error) {
	status, err := server.Ping(ctx, address)
	if err != nil {
		return server.Status{}, err
	}
	if s.store != nil && key != "" {
		_ = s.store.CacheStatus(ctx, key, status)
	}
	return status, nil
}
