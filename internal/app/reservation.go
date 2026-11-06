package app

import (
	"context"

	"mcl/internal/core/launch"
)

// SessionRegistry reserves an instance across launcher processes and rejects
// a verified live session left by a previous launcher. Unknown state is an
// error, never automatic permission to modify its files.
type SessionRegistry interface {
	Reserve(context.Context, string) (SessionLease, error)
}
type SessionLease interface {
	Track(context.Context, launch.Record) error
	Close() error
}

type reservation struct {
	lease   SessionLease
	release func()
}

func (r *reservation) Close() { r.release() }
