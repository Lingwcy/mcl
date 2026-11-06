package install

import (
	"encoding/hex"
	"errors"
	"path"
	"strings"
	"time"

	"mcl/internal/core/instance"
	"mcl/internal/core/meta"
)

// Receipt records a completed installation, not an integrity guarantee for
// later launches. Preparation must still verify the required files each time.
type Receipt struct {
	InstanceID        string           `json:"instanceId"`
	VersionID         string           `json:"versionId"`
	VersionSHA1       string           `json:"versionSha1"`
	OS                string           `json:"os"`
	Arch              string           `json:"arch"`
	NativeDirectory   string           `json:"nativeDirectory"`
	CompletedAt       time.Time        `json:"completedAt"`
	Loader            *instance.Loader `json:"loader,omitempty"`
	LoaderFingerprint string           `json:"loaderFingerprint,omitempty"`
}

func (r Receipt) Validate() error {
	parent := path.Join("instances", r.InstanceID, "natives")
	if !instance.ValidID(r.InstanceID) || !meta.ValidID(r.VersionID) || !meta.ValidSHA1(r.VersionSHA1) || r.CompletedAt.IsZero() || !validNativeGeneration(parent, r.NativeDirectory) {
		return errors.New("invalid installation receipt")
	}
	if (r.OS != "osx" && r.OS != "windows" && r.OS != "linux") || (r.Arch != "amd64" && r.Arch != "arm64" && r.Arch != "386") {
		return errors.New("invalid installation platform")
	}
	if r.Loader != nil {
		if (r.Loader.Type != "fabric" && r.Loader.Type != "quilt") || !meta.ValidID(r.Loader.Version) || len(r.LoaderFingerprint) != 64 {
			return errors.New("invalid installed loader identity")
		}
		if _, err := hex.DecodeString(r.LoaderFingerprint); err != nil {
			return errors.New("invalid installed loader fingerprint")
		}
	} else if r.LoaderFingerprint != "" {
		return errors.New("loader fingerprint has no loader identity")
	}
	return nil
}

func (r Receipt) Matches(value instance.Instance, p meta.Platform) bool {
	if r.Validate() != nil || r.InstanceID != value.ID || r.VersionID != value.Version.ID || !strings.EqualFold(r.VersionSHA1, value.Version.SHA1) || r.OS != p.OS || r.Arch != meta.NormalizeArch(p.Arch) {
		return false
	}
	required, ok := value.RequiredLoader()
	if ok {
		return r.Loader != nil && *r.Loader == required
	}
	return r.Loader == nil
}
