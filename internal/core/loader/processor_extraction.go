package loader

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"mcl/internal/core/archive"
	"mcl/internal/core/meta"
)

// A materialized extraction binds the exact original archive AND its members.
// It is derived in private staging, never persisted under an official digest.
// No unknown descendants are implied by the existence of a directory output.
type processorExtraction struct {
	input     string
	identity  meta.Download
	directory string
	mappings  []archive.Mapping
	files     map[string]meta.Download
}

func mcpMappings(ctx context.Context, z *archive.Zip, p preparedProcessor) ([]archive.Mapping, string, error) {
	options, err := nativeOptions(p)
	if err != nil {
		return nil, "", err
	}
	first := func(key string) string {
		if len(options[key]) != 1 {
			return ""
		}
		return options[key][0].text
	}
	output := first("--output")
	if !forgeOutput(forgeValue{output, true}) || len(p.outputs) != 1 {
		return nil, "", errors.New("MCP任务必须绑定唯一输出根")
	}
	data, err := z.Read(ctx, "config.json", 4<<20)
	if err != nil {
		return nil, "", err
	}
	var config struct{ Data map[string]json.RawMessage }
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, "", err
	}
	keys := strings.Split(first("--key"), ".")
	if len(keys) > 16 {
		return nil, "", errors.New("MCP数据键超过边界")
	}
	values := config.Data
	for index, key := range keys {
		raw, ok := values[key]
		if !ok || key == "" {
			return nil, "", errors.New("MCP数据键不存在")
		}
		if index != len(keys)-1 {
			if err := json.Unmarshal(raw, &values); err != nil {
				return nil, "", err
			}
			continue
		}
		var source string
		if err := json.Unmarshal(raw, &source); err != nil {
			return nil, "", err
		}
		if !strings.HasSuffix(source, "/") {
			if !archive.SafePath(source) || !z.Has(source) {
				return nil, "", errors.New("MCP客户端数据路径无效或不存在")
			}
			return []archive.Mapping{{Source: source, Target: output}}, "", nil
		}
		if !archive.SafePath(strings.TrimSuffix(source, "/")) || p.outputs[output] != "" {
			return nil, "", errors.New("MCP目录声明或摘要无效")
		}
		var mappings []archive.Mapping
		for _, entry := range z.Entries() {
			if err := ctx.Err(); err != nil {
				return nil, "", err
			}
			if entry.Directory || !strings.HasPrefix(entry.Name, source) {
				continue
			}
			target := path.Join(output, strings.TrimPrefix(entry.Name, source))
			if !archive.SafePath(target) {
				return nil, "", errors.New("MCP目录成员目标无效")
			}
			mappings = append(mappings, archive.Mapping{Source: entry.Name, Target: target})
			if len(mappings) > 4096 {
				return nil, "", errors.New("MCP目录输出超过4096项")
			}
		}
		if len(mappings) == 0 {
			return nil, "", errors.New("MCP目录数据为空")
		}
		slices.SortFunc(mappings, func(a, b archive.Mapping) int { return strings.Compare(a.Target, b.Target) })
		return mappings, output, nil
	}
	return nil, "", errors.New("MCP数据键为空")
}

func resolveMCPProcessor(ctx context.Context, stage *os.Root, known map[string]meta.Download, p preparedProcessor, budgets ...*int64) (preparedProcessor, error) {
	if p.nativeTask != "MCP_DATA" {
		return p, nil
	}
	options, err := nativeOptions(p)
	if err != nil {
		return p, err
	}
	if len(options["--input"]) != 1 {
		return p, errors.New("MCP输入必须唯一")
	}
	input := options["--input"][0].text
	fixed, ok := known[input]
	if !ok {
		return p, errors.New("MCP数据归档必须有固定的初始来源，尚不支持运行后才生成的MCP归档")
	}
	if p.extraction != nil {
		if p.extraction.input != input || p.extraction.identity != fixed {
			return p, errors.New("MCP归档身份与已解析计划不符")
		}
		return p, ctx.Err()
	}
	remaining := int64(4 << 30)
	budget := &remaining
	if len(budgets) > 0 && budgets[0] != nil {
		budget = budgets[0]
	}
	plan := &processorExtraction{input: input, identity: fixed, files: make(map[string]meta.Download)}
	err = withForgeArchive(ctx, stage, known, input, func(z *archive.Zip) error {
		mappings, directory, err := mcpMappings(ctx, z, p)
		if err != nil {
			return err
		}
		plan.mappings, plan.directory = mappings, directory
		var total int64
		for _, mapping := range mappings {
			hash := sha1.New()
			size, err := z.Copy(ctx, mapping.Source, hash, min(512<<20, *budget))
			if err != nil {
				return err
			}
			if size <= 0 {
				return errors.New("MCP输出成员必须为非空普通文件")
			}
			*budget -= size
			total += size
			if total > 4<<30 {
				return errors.New("MCP输出累计超过4GiB")
			}
			digest := hex.EncodeToString(hash.Sum(nil))
			if expected := p.outputs[mapping.Target]; expected != "" && !strings.EqualFold(expected, digest) {
				return errors.New("MCP归档成员与固定输出摘要不符")
			}
			plan.files[mapping.Target] = meta.Download{Path: mapping.Target, SHA1: digest, Size: size}
		}
		return ctx.Err()
	})
	if err != nil {
		return p, err
	}
	outputs := make(map[string]string, len(plan.files))
	for name, file := range plan.files {
		outputs[name] = file.SHA1
	}
	p.outputs, p.extraction = outputs, plan
	return p, ctx.Err()
}

// Snapshot and materialize all fixed MCP inputs before any Java resolver or
// processor starts. A later-stage failure must not mutate the caller's plan.
func materializeProcessorPlan(ctx context.Context, stage *os.Root, known map[string]meta.Download, processors []preparedProcessor, runtime []meta.Library) ([]preparedProcessor, error) {
	if err := materializedProcessorBudget(known, nil, runtime); err != nil {
		return nil, err
	}
	remaining := int64(4 << 30)
	prepared := slices.Clone(processors)
	for i, p := range prepared {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p.outputs = maps.Clone(p.outputs)
		p.args = slices.Clone(p.args)
		p.classpath = slices.Clone(p.classpath)
		p.inputs = slices.Clone(p.inputs)
		p.nativeValues = slices.Clone(p.nativeValues)
		var err error
		p, err = resolveMCPProcessor(ctx, stage, known, p, &remaining)
		if err != nil {
			return nil, p.failure("归档计划", err)
		}
		prepared[i] = p
		if err := materializedProcessorBudget(known, prepared[:i+1], runtime); err != nil {
			return nil, err
		}
	}
	if err := materializedProcessorBudget(known, prepared, runtime); err != nil {
		return nil, err
	}
	initial := make(map[string]bool, len(known))
	for name, fixed := range known {
		if !archive.SafePath(name) || fixed.ValidateIdentity() != nil {
			return nil, identityError("暂存初始文件身份无效")
		}
		initial[name] = true
	}
	libraries := make(map[string]forgeLibrary)
	for name, fixed := range known {
		if strings.HasPrefix(name, "libraries/") {
			libraries[strings.TrimPrefix(name, "libraries/")] = forgeLibrary{file: fixed}
		}
	}
	if err := checkProcessorPlan(initial, libraries, prepared, runtime, true); err != nil {
		return nil, err
	}
	// Include work/resources and expanded outputs in the same portable alias
	// check; the declaration-time check alone cannot know MCP member names.
	type node struct {
		name      string
		directory bool
	}
	nodes := make(map[string]node)
	files := make(map[string]bool)
	add := func(name string, directory bool) error {
		if !archive.SafePath(name) {
			return identityError("展开后的处理器路径无效")
		}
		if !directory {
			files[archive.Key(name)] = true
			if len(files) > 8192 {
				return identityError("处理器具体文件计划超过8192项")
			}
		}
		for current := name; current != "."; current = path.Dir(current) {
			key := archive.Key(current)
			if previous, ok := nodes[key]; ok && (previous.name != current || previous.directory != directory) {
				return identityError("展开后的处理器路径存在父目录别名或文件层级冲突")
			}
			nodes[key] = node{current, directory}
			directory = true
			if len(nodes) > 50000 {
				return identityError("处理器目录计划超过5万项")
			}
		}
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(initial)) {
		if err := add(name, false); err != nil {
			return nil, err
		}
	}
	for _, p := range prepared {
		if p.extraction != nil && p.extraction.directory != "" {
			if err := add(p.extraction.directory, true); err != nil {
				return nil, p.failure("归档计划", err)
			}
		}
		for _, name := range slices.Sorted(maps.Keys(p.outputs)) {
			if err := add(name, false); err != nil {
				return nil, p.failure("归档计划", err)
			}
		}
	}
	return prepared, ctx.Err()
}

// Unknown Java output sizes are checked at execution. Proven initial/member
// sizes and every final output path can already be bounded before execution.
func materializedProcessorBudget(known map[string]meta.Download, processors []preparedProcessor, runtime []meta.Library) error {
	planned := make(map[string]meta.Download, len(known))
	maps.Copy(planned, known)
	check := func() error {
		if len(planned) > 8192 {
			return identityError("处理器具体文件计划超过8192项")
		}
		var size int64
		for _, file := range planned {
			if file.Size < 0 || file.Size > 512<<20 {
				return identityError("处理器计划文件大小无效")
			}
			size += file.Size
			if size > 4<<30 {
				return identityError("处理器已知输入及产物计划超过4GiB")
			}
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	for _, p := range processors {
		for name, expected := range p.outputs {
			if p.extraction != nil {
				planned[name] = p.extraction.files[name]
			} else if previous, ok := planned[name]; !ok || !strings.EqualFold(previous.SHA1, expected) {
				planned[name] = meta.Download{Path: name}
			}
		}
		if err := check(); err != nil {
			return p.failure("产物计划", err)
		}
	}
	runtimePaths := make(map[string]bool)
	for _, library := range runtime {
		c, err := meta.ParseCoordinate(library.Name)
		if err != nil {
			return err
		}
		runtimePaths[path.Join("libraries", c.Path())] = true
	}
	outputs := 0
	for name := range planned {
		if strings.HasPrefix(name, "libraries/") && !runtimePaths[name] {
			outputs++
		}
	}
	if outputs > 4096 {
		return identityError("处理器最终附属文件清单超过4096项")
	}
	return nil
}
