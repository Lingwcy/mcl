// Package discovery inspects explicitly chosen Minecraft roots without writes,
// network access, Java execution or adopting foreign files as managed instances.
package discovery

import "mcl/internal/core/meta"

const (
	MaxCandidates    = 2048
	MaxDocumentBytes = 4 << 20
	MaxMetadataBytes = 64 << 20
	MaxProblems      = 64
)

type Loader struct{ Kind, Version string }
type Candidate struct {
	Format                                     string `json:"format,omitempty"`
	ID                                         string
	Folder, DocumentID, GameVersion, MainClass string
	Path, ClientPath                           string
	GamePaths                                  []string
	Loaders                                    []Loader
	Problems                                   []string
	Status                                     string
	MetadataSHA256                             string
	DocumentDigests                            []string
	UnknownComponents                          bool
	Documents                                  []string
}
type Report struct {
	Root          string
	Candidates    []Candidate
	Problems      []string
	Truncated     bool
	DocumentsRead int
	MetadataBytes int64
}

// Parse is deliberately not meta.Client.Version: external JSON and hints are
// untrusted local evidence, not official authenticated launch/install plans.
type document struct {
	ID                 string
	Type               string
	InheritsFrom       string
	Jar                string
	MainClass          string
	MinecraftArguments string
	Patches            []struct{ ID, Version string }
	Libraries          []struct{ Name string }
	Downloads          map[string]meta.Download
}
