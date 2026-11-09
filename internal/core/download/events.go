package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"mcl/internal/core/meta"
)

type Event struct {
	Key, Path, State          string
	Received, Verified, Total int64
}
type observerKey struct{}

func WithObserver(ctx context.Context, observer func(Event) error) context.Context {
	return context.WithValue(ctx, observerKey{}, observer)
}
func Observe(ctx context.Context, event Event) error {
	if fn, ok := ctx.Value(observerKey{}).(func(Event) error); ok {
		return fn(event)
	}
	return ctx.Err()
}
func FileKey(f File) string {
	value := struct {
		Path   string
		Object Object
	}{f.Path, objectForFile(f)}
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func SafeTaskPath(name string) bool { return meta.ValidPath(name) }
func eventFor(f File, state string, received int64) Event {
	e := Event{Key: FileKey(f), Path: f.Path, State: state, Received: received, Total: f.Size}
	if state == "done" {
		e.Verified = f.Size
	}
	return e
}

func Phase(ctx context.Context, stage string) error { return Observe(ctx, Event{State: stage}) }
