package diagnostics

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
)

const MaxArtifacts = 256

// Artifact records only managed relative paths and fixed identity. SHA1 pins
// the containing archive when Member is present; Header is observation only.
type Artifact struct {
	Path, Member, Destination, Role, SHA1, Format, Architectures, State string
	Published                                                           bool `json:",omitempty"`
}

func (a Artifact) Validate() error {
	if a.Published && a.Destination == "" {
		return errors.New("诊断发布目标缺失")
	}
	if a.Path == "" || !SafeSubject(a.Path) || !SafeSubject(a.Member) || !SafeSubject(a.Destination) {
		return errors.New("诊断制品路径无效")
	}
	if a.SHA1 != "" {
		b, e := hex.DecodeString(a.SHA1)
		if e != nil || len(b) != 20 {
			return errors.New("诊断制品摘要无效")
		}
	}
	switch a.Role {
	case "client", "classpath", "native-archive", "native-classpath", "native-binary", "asset-index", "logging", "generated":
	default:
		return errors.New("诊断制品角色无效")
	}
	switch a.State {
	case "planned", "header", "unknown", "unavailable", "limit":
	default:
		return errors.New("诊断制品证据无效")
	}
	switch a.Format {
	case "", "pe", "elf", "macho":
	default:
		return errors.New("诊断二进制格式无效")
	}
	if len(a.Architectures) > 64 {
		return errors.New("诊断架构越界")
	}
	if a.Architectures != "" {
		seen := map[string]bool{}
		for _, arch := range strings.Split(a.Architectures, ".") {
			switch arch {
			case "386", "amd64", "arm", "arm64":
			default:
				return errors.New("诊断架构无效")
			}
			if seen[arch] {
				return errors.New("诊断架构重复")
			}
			seen[arch] = true
		}
	}
	if a.State == "header" && (a.Format == "" || a.Architectures == "") || a.State != "header" && (a.Format != "" || a.Architectures != "") {
		return errors.New("诊断头部证据不完整")
	}
	return nil
}

type artifactKey struct{ Role, Path, Member string }

func (a Artifact) key() artifactKey { return artifactKey{a.Role, a.Path, a.Member} }
func ReportArtifact(ctx context.Context, a Artifact) {
	if o, _ := ctx.Value(observerKey{}).(*observer); o != nil && a.Validate() == nil {
		o.send(Update{Artifact: &a})
	}
}
func (s *Snapshot) applyArtifact(a Artifact) {
	if a.Validate() != nil {
		return
	}
	for i, old := range s.Artifacts {
		if old.key() == a.key() {
			s.Artifacts[i] = a
			return
		}
	}
	if len(s.Artifacts) >= MaxArtifacts {
		s.ArtifactsDropped++
		// Preserve observed headers ahead of the much larger planned classpath.
		if a.Role == "native-binary" {
			for i, old := range s.Artifacts {
				if old.State == "planned" && (old.Role == "classpath" || old.Role == "generated") {
					s.Artifacts[i] = a
					return
				}
			}
		}
		return
	}
	s.Artifacts = append(s.Artifacts, a)
}
func PublishedNatives(ctx context.Context, directory string) {
	if o, _ := ctx.Value(observerKey{}).(*observer); o != nil && directory != "" && SafeSubject(directory) {
		o.send(Update{PublishedDirectory: directory})
	}
}

func (s Snapshot) nativeHeaderSummary() string {
	var observed []string
	for _, a := range s.Artifacts {
		if a.Role == "native-binary" && a.State == "header" {
			value := a.Format + "-" + a.Architectures
			if !slices.Contains(observed, value) {
				observed = append(observed, value)
			}
		}
	}
	slices.Sort(observed)
	return strings.Join(observed, "+")
}

func (s Snapshot) artifactText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "native_abi_scope=retained-headers-only artifacts_dropped=%d\n", s.ArtifactsDropped)
	for _, a := range s.Artifacts {
		fmt.Fprintf(&b, "artifact=%s member=%s destination=%s published=%t role=%s sha1=%s state=%s format=%s arch=%s\n", a.Path, unknown(a.Member), unknown(a.Destination), a.Published, a.Role, unknown(a.SHA1), a.State, unknown(a.Format), unknown(a.Architectures))
	}
	return b.String()
}
