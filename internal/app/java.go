package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"mcl/internal/core/java"
	"mcl/internal/core/meta"
)

type JavaRegistration struct {
	Paths  []string `json:"paths,omitempty"`
	Hidden []string `json:"hidden,omitempty"`
}

type JavaEnvironment struct {
	Runtime java.Runtime
	Kind    string
	Managed bool
}

type JavaInventory struct {
	Environments []JavaEnvironment
	Warnings     []string
}

type JavaRepository interface {
	Load(context.Context) (JavaRegistration, error)
	Register(context.Context, string) error
	Hide(context.Context, string) error
}

type JavaService struct {
	store       JavaRepository
	discover    func(context.Context) ([]string, error)
	installer   *java.Installer
	catalog     *java.CatalogClient
	installSlot chan struct{}
}

func NewJavaService(store JavaRepository, discover func(context.Context) ([]string, error), installer *java.Installer, catalog *java.CatalogClient) *JavaService {
	return &JavaService{store: store, discover: discover, installer: installer, catalog: catalog, installSlot: make(chan struct{}, 1)}
}

func (s *JavaService) Catalog(ctx context.Context, major int, platform meta.Platform) (java.Catalog, error) {
	return s.catalog.Catalog(ctx, major, platform)
}

// Inventory probes at most four distinct files concurrently. Candidates are
// bounded, symlink-resolved and deduplicated by the actual file identity.
func (s *JavaService) Inventory(ctx context.Context, selected string) (JavaInventory, error) {
	return s.inventory(ctx, []string{selected})
}

func (s *JavaService) inventory(ctx context.Context, preferred []string) (JavaInventory, error) {
	registration, err := s.store.Load(ctx)
	if err != nil {
		return JavaInventory{}, err
	}
	paths, err := s.discover(ctx)
	if err != nil {
		return JavaInventory{}, err
	}
	managed, err := java.ManagedCandidates(ctx, s.installer.Directory)
	if err != nil {
		return JavaInventory{}, err
	}
	// Explicit choices come first so broad PATH lists cannot crowd them out.
	paths = append(append(append(slices.Clone(preferred), registration.Paths...), managed...), paths...)
	type candidate struct {
		path string
		info os.FileInfo
	}
	var candidates []candidate
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			continue
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || slices.Contains(registration.Hidden, resolved) {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		duplicate := slices.ContainsFunc(candidates, func(other candidate) bool { return os.SameFile(info, other.info) })
		if !duplicate {
			candidates = append(candidates, candidate{resolved, info})
		}
		if len(candidates) >= 256 {
			break
		}
	}
	result := JavaInventory{}
	var mu sync.Mutex
	var workers sync.WaitGroup
	jobs := make(chan string)
	for range 4 {
		workers.Go(func() {
			for path := range jobs {
				runtime, err := java.Probe(ctx, path)
				mu.Lock()
				if err != nil {
					// Broken registrations remain visible as a report, not as
					// selectable runtimes.
					result.Warnings = append(result.Warnings, path+" · "+err.Error())
				} else {
					result.Environments = append(result.Environments, s.environment(runtime))
				}
				mu.Unlock()
			}
		})
	}
dispatch:
	for _, candidate := range candidates {
		select {
		case jobs <- candidate.path:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	sort.Slice(result.Environments, func(i, j int) bool {
		a, b := result.Environments[i].Runtime, result.Environments[j].Runtime
		if a.Major != b.Major {
			return a.Major > b.Major
		}
		return a.Executable < b.Executable
	})
	// System launcher shims may resolve to an already discovered Java home.
	seen := make(map[string]bool)
	result.Environments = slices.DeleteFunc(result.Environments, func(environment JavaEnvironment) bool {
		if slices.Contains(registration.Hidden, environment.Runtime.Executable) {
			return true
		}
		home := environment.Runtime.Home()
		if canonical, err := filepath.EvalSymlinks(home); err == nil {
			home = canonical
		}
		key := home + "\x00" + environment.Kind
		duplicate := seen[key]
		seen[key] = true
		return duplicate
	})
	sort.Strings(result.Warnings)
	return result, ctx.Err()
}

func (s *JavaService) Add(ctx context.Context, executable string) (JavaEnvironment, error) {
	runtime, err := java.Probe(ctx, executable)
	if err != nil {
		return JavaEnvironment{}, err
	}
	environment := s.environment(runtime)
	if err := s.store.Register(ctx, environment.Runtime.Executable); err != nil {
		return JavaEnvironment{}, err
	}
	return environment, nil
}

func (s *JavaService) Remove(ctx context.Context, executable string) error {
	return s.store.Hide(ctx, executable)
}

func (s *JavaService) Install(ctx context.Context, p java.Package, progress func(java.Progress)) (JavaEnvironment, error) {
	select {
	case s.installSlot <- struct{}{}:
		defer func() { <-s.installSlot }()
	case <-ctx.Done():
		return JavaEnvironment{}, ctx.Err()
	}
	runtime, err := s.installer.Install(ctx, p, progress)
	if err != nil {
		return JavaEnvironment{}, err
	}
	if err := s.store.Register(ctx, runtime.Executable); err != nil {
		return JavaEnvironment{}, errors.New("Java 已安装，但加入列表失败，请刷新后重试")
	}
	return s.environment(runtime), nil
}

func (s *JavaService) environment(runtime java.Runtime) JavaEnvironment {
	// OpenJDK 8 reports a nested jre home even when launched from the JDK.
	home := filepath.Dir(filepath.Dir(runtime.Executable))
	reportedHome := runtime.Home()
	if reportedHome != home && reportedHome != filepath.Join(home, "jre") {
		for _, binary := range []string{"java", "java.exe"} {
			executable, err := filepath.EvalSymlinks(filepath.Join(reportedHome, "bin", binary))
			if err != nil {
				continue
			}
			if info, err := os.Stat(executable); err == nil && info.Mode().IsRegular() {
				runtime.Executable = executable
				home = filepath.Dir(filepath.Dir(executable))
				break
			}
		}
	}
	kind := "JRE"
	for _, name := range []string{"javac", "javac.exe"} {
		if info, err := os.Stat(filepath.Join(home, "bin", name)); err == nil && info.Mode().IsRegular() {
			kind = "JDK"
			runtime.HomeDirectory = home
			break
		}
	}
	relative, err := filepath.Rel(s.installer.Directory, runtime.Executable)
	managed := err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
	return JavaEnvironment{Runtime: runtime, Kind: kind, Managed: managed}
}
