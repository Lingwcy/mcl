package launch

import (
	"errors"
	"path"
	"strings"
	"time"
	"unicode"

	"mcl/internal/core/instance"
	"mcl/internal/core/meta"
)

// ProcessIdentity combines PID with kernel creation identity. It deliberately
// contains no command line, environment, username or credentials.
type ProcessIdentity struct {
	PID   int    `json:"pid"`
	OS    string `json:"os"`
	Stamp string `json:"stamp"`
}

func (p ProcessIdentity) Validate() error {
	if p.PID < 1 || p.PID > 1<<31-1 || (p.OS != "darwin" && p.OS != "linux" && p.OS != "windows") || p.Stamp == "" || len(p.Stamp) > 200 || strings.IndexFunc(p.Stamp, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("-:._", r)
	}) >= 0 {
		return errors.New("invalid process identity")
	}
	return nil
}

// Record is committed before allowing a managed process to execute Java. The
// OS identity survives exec and lets a reopened launcher reject stale PID reuse.
type Record struct {
	ID              string          `json:"id"`
	InstanceID      string          `json:"instanceId"`
	VersionID       string          `json:"versionId"`
	VersionSHA1     string          `json:"versionSha1"`
	NativeDirectory string          `json:"nativeDirectory"`
	Process         ProcessIdentity `json:"process"`
	CreatedAt       time.Time       `json:"createdAt"`
}

func (r Record) Validate() error {
	parent := path.Join("instances", r.InstanceID, "natives")
	if !instance.ValidID(r.ID) || !instance.ValidID(r.InstanceID) || !meta.ValidID(r.VersionID) || !meta.ValidSHA1(r.VersionSHA1) || r.CreatedAt.IsZero() || !meta.ValidPath(r.NativeDirectory) || path.Dir(r.NativeDirectory) != parent || !strings.HasPrefix(path.Base(r.NativeDirectory), "ready-") || len(path.Base(r.NativeDirectory)) != len("ready-")+26 {
		return errors.New("invalid game session record")
	}
	return r.Process.Validate()
}
