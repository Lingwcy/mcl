package loader

import (
	"context"
	"path"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/meta"
)

func reportMavenInput(ctx context.Context, coordinate meta.Coordinate, file meta.Download, role, state, local string) {
	if !diagnostics.Observed(ctx) {
		return
	}
	repository := "embedded"
	if file.URL != "" {
		location, err := meta.BindMavenURL(file.URL, coordinate)
		if err != nil {
			return
		}
		repository = location.Repository
	}
	diagnostics.ReportArtifact(ctx, diagnostics.Artifact{Path: local, Role: role, Coordinate: coordinate.String(), Repository: repository, SHA1: file.SHA1, State: state})
}
func reportLoaderGeneration(ctx context.Context, profile Profile) { profile.ReportEvidence(ctx) }

// ReportEvidence describes validated profile values only. It performs no I/O.
func (profile Profile) ReportEvidence(ctx context.Context) {
	if !diagnostics.Observed(ctx) || profile.Validate() != nil {
		return
	}
	if !UsesProcessors(profile.Request.Type) {
		for _, library := range profile.Libraries {
			if library.Downloads.Artifact == nil {
				continue
			}
			coordinate, err := meta.ParseCoordinate(library.Name)
			if err != nil {
				continue
			}
			reportMavenInput(ctx, coordinate, *library.Downloads.Artifact, "loader-dependency", "planned", "cache/libraries/"+library.Downloads.Artifact.Path)
		}
		return
	}
	for _, library := range profile.Libraries {
		coordinate, err := meta.ParseCoordinate(library.Name)
		if err != nil || library.Downloads.Artifact == nil {
			continue
		}
		file := *library.Downloads.Artifact
		local := path.Join(profile.LibraryDirectory(), file.Path)
		diagnostics.ReportArtifact(ctx, diagnostics.Artifact{Path: local, Destination: local, Published: true, Role: "loader-runtime", Coordinate: coordinate.String(), Repository: "managed-generation", SHA1: file.SHA1, State: "verified"})
	}
}
