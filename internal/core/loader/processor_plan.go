package loader

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/diagnostics"
	"mcl/internal/core/meta"
)

// ProcessorError retains the original installer position and a bounded
// canonical tool identity. Causes remain unwrap-able for cancellation/IO codes.
type ProcessorError struct {
	Position    int
	Tool, Stage string
	Cause       error
}

func (e *ProcessorError) Error() string {
	return fmt.Sprintf("安装处理器 %d (%s)，%s: %v", e.Position, e.Tool, e.Stage, e.Cause)
}
func (e *ProcessorError) Unwrap() error { return e.Cause }
func processorFailure(position int, tool, stage string, cause error) error {
	if cause == nil {
		return nil
	}
	coordinate, err := meta.ParseCoordinate(tool)
	label := "未知工具"
	if err == nil && diagnostics.SafeSubject(coordinate.String()) {
		label = coordinate.String()
	}
	return &ProcessorError{Position: position, Tool: label, Stage: stage, Cause: cause}
}
func (p preparedProcessor) failure(stage string, err error) error {
	return processorFailure(p.position, p.coordinate, stage, err)
}

// validateProcessorPlan only examines fixed declarations and ZIP inventory. It
// does not download, extract, probe Java, execute code or claim output success.
// A directory MCP output is conditional until its fixed archive is read; actual
// members and digests MUST still be established by the execution-time checks.
func validateProcessorPlan(z *archive.Zip, libraries map[string]forgeLibrary, processors []preparedProcessor, resources []archive.Mapping, runtime []meta.Library) error {
	available := map[string]bool{"work/installer.jar": true, "work/client.jar": true}
	initial := map[string]bool{"work/installer.jar": true, "work/client.jar": true}
	trees := make(map[string]bool)
	directories := make(map[string]bool)
	tools := make(map[string]bool)
	addSource := func(name string) { available[name] = true; initial[name] = true }
	for key, library := range libraries {
		if library.file.URL != "" || z.Has(path.Join("maven", key)) {
			addSource(path.Join("libraries", key))
		}
	}
	for _, resource := range resources {
		if !z.Has(resource.Source) {
			return identityError("安装器缺少已声明的内嵌资源")
		}
		if !archive.SafePath(resource.Target) {
			return identityError("安装器内嵌资源目标无效")
		}
		addSource(resource.Target)
	}
	for _, p := range processors {
		if p.nativeTask != "" || p.copyMappings != "" {
			continue
		}
		for _, name := range p.classpath {
			if !initial[name] {
				return p.failure("工具依赖", identityError("classpath缺少固定初始来源: "+name))
			}
			tools[name] = true
		}
	}
	present := func(name string) bool {
		if available[name] || trees[name] || directories[name] {
			return true
		}
		for directory := range trees {
			if strings.HasPrefix(name, directory+"/") {
				return true
			}
		}
		return false
	}
	for _, p := range processors {
		for _, name := range p.inputs {
			if !present(name) {
				return p.failure("输入计划", identityError("输入没有初始来源或前置产物: "+name))
			}
		}
		for target := range p.outputs {
			for tool := range tools {
				if processorPathsOverlap(target, tool) {
					return p.failure("输出计划", identityError("输出与固定工具classpath重叠: "+target))
				}
			}
			for source := range initial {
				if source != target && processorPathsOverlap(source, target) {
					return p.failure("输出计划", identityError("输出与固定输入文件层级重叠: "+target))
				}
			}
			if initial[target] && !slices.Contains(p.inputs, target) && !optionalProcessorOutput(p, target) {
				expected := p.outputs[target]
				fixed := libraries[strings.TrimPrefix(target, "libraries/")].file
				if expected == "" || !strings.EqualFold(expected, fixed.SHA1) {
					return p.failure("输出计划", identityError("输出覆盖未作为输入的固定文件: "+target))
				}
			}
		}
		for target, expected := range p.outputs {
			available[target] = true
			if p.nativeTask == "MCP_DATA" && expected == "" {
				trees[target] = true
			} else {
				delete(trees, target)
			}
		}
		if p.nativeTask == "CREATE_DIR" || p.nativeTask == "CREATE_PARENTS" {
			options, err := nativeOptions(p)
			if err != nil {
				return p.failure("目录计划", err)
			}
			for _, value := range options["--target"] {
				directory := value.text
				if p.nativeTask == "CREATE_PARENTS" {
					directory = path.Dir(directory)
				}
				if directory != "." && directory != "libraries" && directory != "work" {
					directories[directory] = true
				}
			}
		}
	}
	for _, library := range runtime {
		coordinate, err := meta.ParseCoordinate(library.Name)
		if err != nil {
			return identityError("运行库坐标无效")
		}
		if !present(path.Join("libraries", coordinate.Path())) {
			return identityError("运行库没有初始来源或处理器产物: " + coordinate.String())
		}
	}
	return nil
}
func processorPathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

// Keep a shared sentinel for callers that need to distinguish an actual hash
// mismatch from cancellation, read errors or a file identity/size race.
var errForgeDigestMismatch = errors.New("安装文件摘要与固定声明不符")
