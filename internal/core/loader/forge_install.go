package loader

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/diagnostics"
	"mcl/internal/core/download"
	"mcl/internal/core/java"
	"mcl/internal/core/meta"
	"mcl/internal/fileio"
)

type ProcessorRunner func(context.Context, string, string, []string) error

// ProcessorJavaResolver returns a probed stable runtime for this tool minimum.
// It is deliberately separate from the game runtime and cannot change its major.
type ProcessorJavaResolver func(context.Context, int, meta.Platform, []string, func(java.Progress)) (java.Runtime, error)

type ForgeInstaller struct {
	catalog     *Client
	downloads   *download.Client
	run         ProcessorRunner
	resolveJava ProcessorJavaResolver
}

func NewForgeInstaller(catalog *Client, downloads *download.Client, run ProcessorRunner) *ForgeInstaller {
	return &ForgeInstaller{catalog: catalog, downloads: downloads, run: run}
}

func NewForgeInstallerWithJava(catalog *Client, downloads *download.Client, run ProcessorRunner, resolve ProcessorJavaResolver) *ForgeInstaller {
	s := NewForgeInstaller(catalog, downloads, run)
	s.resolveJava = resolve
	return s
}

func (s *ForgeInstaller) Prepare(ctx context.Context, request Request, base meta.Version, executable string, platform meta.Platform, report func(string, download.Progress)) (Profile, error) {
	// Downloads retain their active-network budgets; user rate-limit waits and
	// automatic tool-Java installation are not charged to one wall-clock timer.
	// The production processor adapter bounds each actual child separately.
	if err := ctx.Err(); err != nil {
		return Profile{}, err
	}
	if request.TargetOS != "" && (request.TargetOS != platform.OS || request.TargetArch != meta.NormalizeArch(platform.Arch)) {
		return Profile{}, errors.New("安装器目标平台与缓存身份不符")
	}
	if err := request.ValidateBase(base); err != nil {
		return Profile{}, err
	}
	if err := base.ValidateVanilla(); err != nil {
		return Profile{}, err
	}
	coordinate, origin, versionID, err := forgeInstaller(request)
	if err != nil {
		return Profile{}, err
	}
	if base.ID != request.GameVersion {
		return Profile{}, errors.New("安装处理器的 Minecraft 版本不匹配")
	}
	relative := coordinate.Path()
	installer, err := s.catalog.mavenMetadata(ctx, origin+"/"+relative, relative)
	if err != nil {
		return Profile{}, fmt.Errorf("读取官方安装包摘要和大小: %w", err)
	}
	bundle := forgeInput(installer)
	progress := func(stage string) func(download.Progress) {
		return func(p download.Progress) {
			if report != nil {
				report(stage, p)
			}
		}
	}
	if err := s.downloads.Run(ctx, []download.File{bundle}, progress("loader-download")); err != nil {
		return Profile{}, err
	}
	cache, err := os.OpenRoot(s.downloads.Root)
	if err != nil {
		return Profile{}, err
	}
	defer cache.Close()
	parentName := path.Join("cache/loader-runtimes", request.Key())
	if err := cache.MkdirAll(parentName, 0o700); err != nil {
		return Profile{}, err
	}
	parent, err := cache.OpenRoot(parentName)
	if err != nil {
		return Profile{}, err
	}
	defer parent.Close()
	nonce := rand.Text()
	temporary := ".mcl-prepare-" + nonce
	if err := parent.Mkdir(temporary, 0o700); err != nil {
		return Profile{}, err
	}
	defer parent.RemoveAll(temporary)
	stage, err := parent.OpenRoot(temporary)
	if err != nil {
		return Profile{}, err
	}
	defer stage.Close()
	if err := copyForgeInput(ctx, cache, bundle, stage, "work/installer.jar"); err != nil {
		return Profile{}, err
	}
	bundleFile, err := stage.Open("work/installer.jar")
	if err != nil {
		return Profile{}, err
	}
	defer bundleFile.Close()
	z, err := archive.Open(bundleFile, bundle.Size)
	if err != nil {
		return Profile{}, err
	}
	legacy, isLegacy, err := readLegacyForgeSpec(ctx, z, request, base)
	if err != nil {
		return Profile{}, err
	}
	if isLegacy {
		profile, err := s.prepareLegacyForge(ctx, z, request, base, legacy, stage, "ready-"+nonce, bundle.SHA1, progress("loader-download"))
		if err != nil {
			return Profile{}, err
		}
		if err := bundleFile.Close(); err != nil {
			return Profile{}, err
		}
		if err := stage.RemoveAll("work"); err != nil {
			return Profile{}, err
		}
		if err := ctx.Err(); err != nil {
			return Profile{}, err
		}
		if err := stage.Close(); err != nil {
			return Profile{}, err
		}
		if err := fileio.PublishDirectory(parent, temporary, profile.Generation); err != nil {
			return Profile{}, err
		}
		return profile, nil
	}
	spec, version, err := readForgeSpec(ctx, z, request, versionID)
	if err != nil {
		return Profile{}, err
	}
	if _, declared := spec.Data["BINPATCH"]; !declared && z.Has("data/client.lzma") {
		if spec.Data == nil {
			spec.Data = make(map[string]struct{ Client string })
		}
		spec.Data["BINPATCH"] = struct{ Client string }{Client: "/data/client.lzma"}
	}
	values, resources, err := forgeData(spec)
	if err != nil {
		return Profile{}, err
	}
	stagePath := filepath.Join(s.downloads.Root, filepath.FromSlash(parentName), temporary)
	if strings.ContainsRune(stagePath, os.PathListSeparator) {
		return Profile{}, errors.New("加载器目录包含 Java 类路径分隔符")
	}
	var processors []preparedProcessor
	for _, entry := range spec.Processors {
		p, err := prepareForgeProcessor(entry, values, stagePath)
		if err != nil {
			return Profile{}, err
		}
		processors = append(processors, p)
	}
	libraries, err := forgeLibraries(spec.Libraries, version.Libraries)
	if err != nil {
		return Profile{}, err
	}
	if err := validateProcessorPaths(libraries, processors); err != nil {
		return Profile{}, err
	}
	clientID := base.ID
	if base.ClientVersion != "" {
		clientID = base.ClientVersion
	}
	client := download.File{Path: path.Join("cache/versions", clientID, clientID+".jar"), Download: base.Downloads["client"]}
	inputs, copies, err := planForgeInputs(z, libraries, processors, base, client)
	if err != nil {
		return Profile{}, err
	}
	if err := s.downloads.Run(ctx, inputs, progress("loader-download")); err != nil {
		return Profile{}, err
	}
	if err := z.Extract(ctx, stage, append(resources, copies...)); err != nil {
		return Profile{}, err
	}
	if err := copyForgeInput(ctx, cache, client, stage, "work/client.jar"); err != nil {
		return Profile{}, err
	}
	known := make(map[string]meta.Download, len(libraries))
	for _, key := range slices.Sorted(maps.Keys(libraries)) {
		file := libraries[key].file
		target := path.Join("libraries", key)
		switch {
		case z.Has(path.Join("maven", key)):
		case file.URL != "":
			if err := copyForgeInput(ctx, cache, forgeInput(file), stage, target); err != nil {
				return Profile{}, fmt.Errorf("准备库 %s: %w", key, err)
			}
		default:
			continue
		}
		identity, err := hashForgeFile(ctx, stage, target, file.SHA1, file.Size)
		if err != nil {
			return Profile{}, err
		}
		known[target] = identity
	}
	known["work/installer.jar"] = installer
	known["work/client.jar"] = client.Download
	for _, resource := range resources {
		identity, err := hashForgeFile(ctx, stage, resource.Target, "", 0)
		if err != nil {
			return Profile{}, err
		}
		known[resource.Target] = identity
	}
	if err := bundleFile.Close(); err != nil {
		return Profile{}, err
	}
	if err := s.runProcessors(ctx, cache, stage, stagePath, executable, platform, base, processors, known, progress("loader-processors"), func(p java.Progress) {
		if report != nil {
			report(p.Stage, download.Progress{Bytes: p.Bytes, TotalBytes: p.Total})
		}
	}); err != nil {
		return Profile{}, err
	}
	if patched, ok := values["PATCHED"]; ok {
		if _, exists := known[patched.text]; !patched.file || !exists {
			return Profile{}, errors.New("安装处理器缺少客户端补丁产物")
		}
	}
	for _, key := range slices.Sorted(maps.Keys(known)) {
		identity := known[key]
		if _, err := hashForgeFile(ctx, stage, key, identity.SHA1, identity.Size); err != nil {
			return Profile{}, err
		}
	}
	profile, err := completedForgeProfile(request, version, known, "ready-"+nonce, bundle.SHA1, spec.LegacyGame)
	if err != nil {
		return Profile{}, err
	}
	if _, err := profile.Merge(base); err != nil {
		return Profile{}, err
	}
	if err := verifyProcessedLegacyEntrypoints(ctx, stage, profile); err != nil {
		return Profile{}, err
	}
	if err := stage.RemoveAll("work"); err != nil {
		return Profile{}, err
	}
	if err := ctx.Err(); err != nil {
		return Profile{}, err
	}
	if err := stage.Close(); err != nil {
		return Profile{}, err
	}
	if err := fileio.PublishDirectory(parent, temporary, profile.Generation); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func forgeInput(file meta.Download) download.File {
	return download.File{Path: "cache/loader-inputs/" + strings.ToLower(file.SHA1) + ".bin", AllowedHosts: LibraryHosts(), Download: file}
}

func forgeMappings(base meta.Version) download.File {
	return download.File{
		Path:         path.Join("cache/versions", base.ID, base.ID+"-client-mappings.txt"),
		AllowedHosts: []string{"piston-data.mojang.com", "launcher.mojang.com", "launchermeta.mojang.com"},
		Download:     base.Downloads["client_mappings"],
	}
}

func planForgeInputs(z *archive.Zip, libraries map[string]forgeLibrary, processors []preparedProcessor, base meta.Version, client download.File) ([]download.File, []archive.Mapping, error) {
	inputs := []download.File{client}
	var mappings []archive.Mapping
	for _, key := range slices.Sorted(maps.Keys(libraries)) {
		lib := libraries[key]
		entry := path.Join("maven", key)
		if z.Has(entry) {
			mappings = append(mappings, archive.Mapping{Source: entry, Target: path.Join("libraries", key)})
		} else if lib.file.URL != "" {
			inputs = append(inputs, forgeInput(lib.file))
		}
	}
	for _, p := range processors {
		if p.copyMappings != "" {
			inputs = append(inputs, forgeMappings(base))
			break
		}
	}
	var total int64
	for _, f := range inputs {
		total += f.Size
		if total > 4<<30 {
			return nil, nil, errors.New("安装处理器下载超过 4 GiB")
		}
	}
	return inputs, mappings, nil
}

func (s *ForgeInstaller) runProcessors(ctx context.Context, cache, stage *os.Root, stagePath, executable string, platform meta.Platform, base meta.Version, processors []preparedProcessor, known map[string]meta.Download, report func(download.Progress), javaReport func(java.Progress)) (resultErr error) {
	classes := 0
	runtimes := make(map[int]java.Runtime)
	progress := download.Progress{TotalFiles: len(processors)}
	report(progress)
	for index, p := range processors {
		ctx, step := diagnostics.Start(ctx, "processor", p.jar)
		step.Step(index+1, len(processors))
		defer func() {
			if resultErr != nil {
				step.Fail(resultErr)
			}
			step.End()
		}()
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := verifyProcessorInputs(ctx, stage, known, p.inputs); err != nil {
			return err
		}
		reused, err := prepareProcessorOutputs(ctx, stage, known, p)
		if err != nil {
			return err
		}
		if reused {
			step.Work()
			step.End()
			progress.Files++
			report(progress)
			continue
		}
		if p.copyMappings != "" {
			if err := prepareForgeMappings(ctx, cache, stage, base, p); err != nil {
				return err
			}
		} else if p.nativeTask != "" {
			if err := runNativeProcessor(ctx, stage, known, p); err != nil {
				return err
			}
		} else {
			if s.run == nil {
				return errors.New("Java 安装处理器未配置")
			}
			main, err := forgeJarMain(ctx, stage, p.jar, p.classpath)
			if err != nil {
				return err
			}
			// Modern installers run with at least the official game JVM baseline.
			// Java 8 class headers only describe the bytecode floor; they do not
			// require a Java 8 process for a Java 17/21 game installation. Keep
			// the existing exact-8 policy for historical game/tool combinations.
			minimum := max(8, base.Java.Major)
			var classpath []string
			for _, relative := range p.classpath {
				file, ok := known[relative]
				if !ok {
					return fmt.Errorf("安装处理器缺少已校验依赖 %s", relative)
				}
				entry := ""
				if relative == p.jar {
					entry = main
				}
				required, err := processorJarMinimum(ctx, stage, relative, file, entry, &classes, p.classpath)
				if err != nil {
					return fmt.Errorf("分析安装工具Java要求 %s: %w", relative, err)
				}
				minimum = max(minimum, required)
				classpath = append(classpath, filepath.Join(stagePath, filepath.FromSlash(relative)))
			}
			runtime, found := runtimes[minimum]
			if !found {
				runtime, err = s.processorRuntime(ctx, minimum, platform, []string{executable}, javaReport)
				if err != nil {
					return err
				}
				runtimes[minimum] = runtime
			}
			diagnostics.SetFacts(ctx, runtime.DiagnosticFacts(true))
			// Java installation may take time; recheck the fixed tools immediately
			// before handing their paths to the process adapter.
			for _, relative := range p.classpath {
				file := known[relative]
				if _, err := hashForgeFile(ctx, stage, relative, file.SHA1, file.Size); err != nil {
					return err
				}
			}
			if err := verifyProcessorInputs(ctx, stage, known, p.inputs); err != nil {
				return err
			}
			args := append([]string{"-Xmx1024m", "-Dfile.encoding=UTF-8", "-Djava.net.preferIPv4Stack=true", "-cp",
				strings.Join(classpath, string(os.PathListSeparator)), main}, p.args...)
			if err := s.run(ctx, runtime.Executable, filepath.Join(stagePath, "work"), args); err != nil {
				return fmt.Errorf("安装处理器 %d (%s): %w", index+1, path.Base(p.jar), err)
			}
		}
		for target, expected := range p.outputs {
			identity, err := hashForgeFile(ctx, stage, target, expected, 0)
			if err != nil {
				return err
			}
			known[target] = identity
		}
		var generatedBytes int64
		for _, identity := range known {
			generatedBytes += identity.Size
			if generatedBytes > 4<<30 {
				return errors.New("处理器输入及生成文件累计超过4GiB")
			}
		}
		step.Work()
		step.End()
		progress.Files++
		report(progress)
	}
	return nil
}

func completedForgeProfile(request Request, version meta.Version, known map[string]meta.Download, generation, installerSHA1 string, legacyGame bool) (Profile, error) {
	profile := Profile{Request: request, ID: version.ID, MainClass: version.MainClass, JavaMajor: version.Java.Major,
		Arguments: *version.Arguments, Generation: generation, InstallerSHA1: installerSHA1, LegacyGame: legacyGame}
	seen := make(map[string]bool)
	for _, library := range version.Libraries {
		coordinate, err := meta.ParseCoordinate(library.Name)
		if err != nil {
			return Profile{}, err
		}
		target := path.Join("libraries", coordinate.Path())
		seen[target] = true
		identity, ok := known[target]
		if !ok {
			return Profile{}, fmt.Errorf("启动所需库缺少安装产物 %s", library.Name)
		}
		if expected := library.Downloads.Artifact; expected != nil && expected.SHA1 != "" &&
			(!strings.EqualFold(expected.SHA1, identity.SHA1) || expected.Size > 0 && expected.Size != identity.Size) {
			return Profile{}, fmt.Errorf("启动库与固定摘要不匹配 %s", library.Name)
		}
		identity.Path = coordinate.Path()
		library.Downloads.Artifact = &identity
		profile.Libraries = append(profile.Libraries, library)
	}
	for _, target := range slices.Sorted(maps.Keys(known)) {
		if !seen[target] && strings.HasPrefix(target, "libraries/") {
			identity := known[target]
			identity.Path = strings.TrimPrefix(target, "libraries/")
			profile.Outputs = append(profile.Outputs, identity)
		}
	}
	return profile, profile.Validate()
}

func (p Profile) VerifyRuntime(ctx context.Context, root *os.Root) error {
	if !UsesProcessors(p.Request.Type) {
		return nil
	}
	libraries, err := root.OpenRoot(p.LibraryDirectory())
	if err != nil {
		return err
	}
	defer libraries.Close()
	for _, library := range p.Libraries {
		f := library.Downloads.Artifact
		if _, err := hashForgeFile(ctx, libraries, f.Path, f.SHA1, f.Size); err != nil {
			return err
		}
	}
	for _, output := range p.Outputs {
		if _, err := hashForgeFile(ctx, libraries, output.Path, output.SHA1, output.Size); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func copyForgeInput(ctx context.Context, cache *os.Root, input download.File, stage *os.Root, target string) error {
	if err := stage.MkdirAll(path.Dir(target), 0o700); err != nil {
		return err
	}
	before, err := cache.Lstat(input.Path)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() || before.Size() != input.Size {
		return errors.New("安装处理器缓存类型或大小无效")
	}
	source, err := cache.Open(input.Path)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, info) || info.Size() != input.Size {
		return errors.New("安装处理器缓存类型或大小无效")
	}
	destination, err := stage.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	size, err := io.Copy(destination, forgeReader{ctx, io.LimitReader(source, input.Size+1)})
	if err := errors.Join(err, destination.Close()); err != nil {
		return err
	}
	if size != input.Size {
		return errors.New("安装处理器缓存大小已改变")
	}
	_, err = hashForgeFile(ctx, stage, target, input.SHA1, input.Size)
	return err
}

type forgeReader struct {
	ctx context.Context
	io.Reader
}

func (r forgeReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(buffer)
}

func hashForgeFile(ctx context.Context, root *os.Root, name, expected string, expectedSize int64) (meta.Download, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return meta.Download{}, err
	}
	size := before.Size()
	if !before.Mode().IsRegular() || size <= 0 || size > 512<<20 || (expectedSize > 0 && size != expectedSize) {
		return meta.Download{}, fmt.Errorf("安装处理器文件类型或大小无效 %s", name)
	}
	f, err := root.Open(name)
	if err != nil {
		return meta.Download{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !os.SameFile(before, info) {
		return meta.Download{}, fmt.Errorf("安装处理器文件已改变 %s", name)
	}
	hash := sha1.New()
	n, err := io.Copy(hash, forgeReader{ctx, io.LimitReader(f, size+1)})
	if err != nil {
		return meta.Download{}, err
	}
	current, err := root.Lstat(name)
	sum := hex.EncodeToString(hash.Sum(nil))
	if err != nil || n != size || !os.SameFile(before, current) || current.Size() != size || !current.ModTime().Equal(before.ModTime()) ||
		(expected != "" && !strings.EqualFold(sum, expected)) {
		return meta.Download{}, fmt.Errorf("安装处理器文件已改变或摘要不符 %s", name)
	}
	return meta.Download{Path: name, SHA1: sum, Size: size}, ctx.Err()
}
