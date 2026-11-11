package install

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"strings"

	"mcl/internal/core/diagnostics"
	"mcl/internal/core/download"
	"mcl/internal/core/meta"
	"mcl/internal/core/nativeabi"
)

// reportPlan describes roles from the actual selected plan, not filenames of
// arbitrary mods. Planned identity is not a claim of publication or loaded code.
func reportPlan(ctx context.Context, p Plan) {
	if !diagnostics.Observed(ctx) {
		return
	}
	known := make(map[string]bool)
	for _, library := range p.Libraries {
		role := string(library.Role)
		repository := ""
		if library.Local {
			repository = "managed-generation"
		} else if location, err := meta.ParseMavenURL(library.File.URL); err == nil {
			repository = location.Repository
		}
		if library.Role == LibraryRuntimeLookup || library.Local && library.Role == LibraryClasspath {
			role = "generated"
		}
		diagnostics.ReportArtifact(ctx, diagnostics.Artifact{Path: library.File.Path, Role: role, Coordinate: library.Coordinate, Repository: repository, SHA1: library.File.SHA1, State: "planned"})
		known[library.File.Path] = true
	}
	for _, file := range p.Files {
		if known[file.Path] {
			continue
		}
		role := "classpath"
		switch file.Path {
		case p.ClientPath:
			role = "client"
		case p.AssetIndexPath:
			role = "asset-index"
		case p.LoggingPath:
			role = "logging"
		}
		diagnostics.ReportArtifact(ctx, diagnostics.Artifact{Path: file.Path, Role: role, SHA1: file.SHA1, State: "planned"})
	}
	if p.Graphics.Family != "" && p.Graphics.Family != "unobserved" {
		_, span := diagnostics.Start(ctx, "metadata", "graphics-plan:"+p.Graphics.Family)
		span.End()
	}
}

// Header observation is diagnostic only. Errors do not turn into an additional
// installer failure path. Full archive identity is verified before and after
// reading through the same FD; partial ZIP-member headers are never described
// as CRC validation, full binary validation or dynamic linkage evidence.
type nativeEvidenceKey struct{}
type nativeEvidenceBudget struct {
	archives               int
	hashBytes, headerBytes int64
}

func withNativeEvidenceBudget(ctx context.Context) context.Context {
	if ctx.Value(nativeEvidenceKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, nativeEvidenceKey{}, &nativeEvidenceBudget{})
}

func observeNativeArchive(ctx context.Context, root *os.Root, file download.File, exclude []string, destination string) {
	if !diagnostics.Observed(ctx) {
		return
	}
	ctx, span := diagnostics.Start(ctx, "verify", file.Path)
	defer span.End()
	budget, _ := ctx.Value(nativeEvidenceKey{}).(*nativeEvidenceBudget)
	if budget == nil {
		budget = &nativeEvidenceBudget{}
	}
	if budget.archives >= 128 || file.Size < 0 || file.Size > 256<<20-budget.hashBytes || budget.headerBytes >= 64<<20 {
		diagnostics.ReportArtifact(ctx, diagnostics.Artifact{Path: file.Path, Role: "native-binary", SHA1: file.SHA1, State: "limit"})
		return
	}
	budget.archives++
	budget.hashBytes += file.Size
	failure := diagnostics.Artifact{Path: file.Path, Role: "native-binary", SHA1: file.SHA1, State: "unavailable"}
	observations, err := nativeHeaders(ctx, root, file, exclude, destination, budget)
	if err != nil {
		span.Fail(err)
		diagnostics.ReportArtifact(ctx, failure)
		return
	}
	span.Work()
	for _, a := range observations {
		diagnostics.ReportArtifact(ctx, a)
	}
}
func nativeHeaders(ctx context.Context, root *os.Root, file download.File, exclude []string, destination string, budget *nativeEvidenceBudget) ([]diagnostics.Artifact, error) {
	f, before, err := openAssetFile(root, file.Path, file.Size)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := streamAsset(ctx, f, io.Discard, file.Size, file.SHA1); err != nil {
		return nil, err
	}
	z, err := zip.NewReader(f, file.Size)
	if err != nil {
		return nil, err
	}
	if len(z.File) > 10000 {
		return []diagnostics.Artifact{{Path: file.Path, Role: "native-binary", SHA1: file.SHA1, State: "limit"}}, nil
	}
	var out []diagnostics.Artifact
	var readBytes int64
	for _, entry := range z.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := entry.Name
		if entry.FileInfo().IsDir() || entry.Mode()&os.ModeType != 0 || !diagnostics.SafeSubject(name) || excludedNative(name, exclude) || !nativeFilename(name) {
			continue
		}
		if len(out) >= 64 || readBytes >= 8<<20 || budget.headerBytes >= 64<<20 {
			out = append(out, diagnostics.Artifact{Path: file.Path, Role: "native-binary", SHA1: file.SHA1, State: "limit"})
			break
		}
		a := diagnostics.Artifact{Path: file.Path, Member: name, Role: "native-binary", SHA1: file.SHA1, State: "unknown"}
		if destination != "" {
			a.Destination = destination + "/" + name
		}
		reader, err := entry.Open()
		if err != nil {
			return nil, err
		}
		limit := min(int64(nativeabi.MaxHeader), 8<<20-readBytes, 64<<20-budget.headerBytes)
		data, readErr := io.ReadAll(io.LimitReader(assetReader{ctx, reader}, limit))
		closeErr := reader.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		readBytes += int64(len(data))
		budget.headerBytes += int64(len(data))
		image := nativeabi.Inspect(bytes.NewReader(data), int64(min(entry.UncompressedSize64, 1<<63-1)))
		if image.Format != "" {
			a.Format, a.Architectures, a.State = image.Format, image.Architectures, "header"
		}
		out = append(out, a)
	}
	if err := streamAsset(ctx, f, io.Discard, file.Size, file.SHA1); err != nil {
		return nil, err
	}
	if err := sameAssetFile(root, file.Path, f, before); err != nil {
		return nil, err
	}
	return out, nil
}
func nativeFilename(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".dll") || strings.HasSuffix(lower, ".dylib") || strings.HasSuffix(lower, ".jnilib") || strings.HasSuffix(lower, ".so") || strings.Contains(lower, ".so.")
}
func excludedNative(name string, exclude []string) bool {
	if name == "META-INF" || strings.HasPrefix(name, "META-INF/") {
		return true
	}
	for _, prefix := range exclude {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
