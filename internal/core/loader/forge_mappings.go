package loader

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"mcl/internal/core/meta"
)

type pgMember struct{ name, mapped, desc string }
type pgClass struct {
	name, mapped    string
	fields, methods map[string]pgMember
}

func prepareForgeMappings(ctx context.Context, cache, stage *os.Root, base meta.Version, p preparedProcessor) error {
	fixed := forgeMappings(base)
	if !p.sanitizeMappings {
		return copyForgeInput(ctx, cache, fixed, stage, p.copyMappings)
	}
	if fixed.Size <= 0 || fixed.Size > 64<<20 {
		return errors.New("官方映射超过64MiB")
	}
	source := "work/mojmaps-" + strings.ToLower(fixed.SHA1) + ".txt"
	if _, err := hashForgeFile(ctx, stage, source, fixed.SHA1, fixed.Size); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := copyForgeInput(ctx, cache, fixed, stage, source); err != nil {
			return err
		}
	}
	f, err := stage.Open(source)
	if err != nil {
		return err
	}
	before, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	raw, readErr := io.ReadAll(forgeReader{ctx, io.LimitReader(f, fixed.Size+1)})
	after, statErr := f.Stat()
	f.Close()
	current, pathErr := stage.Lstat(source)
	if err := errors.Join(readErr, statErr, pathErr); err != nil {
		return err
	}
	if len(raw) != int(fixed.Size) || !strings.EqualFold(hex.EncodeToString(sumSHA1(raw)), fixed.SHA1) || !current.Mode().IsRegular() || !os.SameFile(before, current) || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		return errors.New("官方映射读取期间发生变化")
	}
	classes, err := parseProguard(ctx, raw)
	if err != nil {
		return err
	}
	expected := p.outputs[p.copyMappings]
	// Different published mapping-library generations used different field order.
	// Neither is accepted by guess: the installer's fixed output SHA1 chooses it.
	var data []byte
	for _, mappedOrder := range []bool{true, false} {
		candidate, err := writeTSRG(ctx, classes, p.mappingFormat, mappedOrder)
		if err != nil {
			return err
		}
		if strings.EqualFold(hex.EncodeToString(sumSHA1(candidate)), expected) {
			data = candidate
			break
		}
	}
	if data == nil {
		return errors.New("官方映射规范化与安装器固定输出摘要不符")
	}
	output, err := stage.OpenFile(p.copyMappings, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := io.Copy(output, forgeReader{ctx, bytes.NewReader(data)})
	return errors.Join(writeErr, output.Close(), ctx.Err())
}
func sumSHA1(data []byte) []byte { sum := sha1.Sum(data); return sum[:] }

func parseProguard(ctx context.Context, raw []byte) ([]*pgClass, error) {
	if len(raw) > 64<<20 || !utf8.Valid(raw) {
		return nil, errors.New("官方映射大小或编码无效")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	byName := make(map[string]*pgClass)
	var current *pgClass
	lines, members := 0, 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lines++
		if lines > 1_000_000 {
			return nil, errors.New("官方映射行数超过边界")
		}
		source, _, _ := strings.Cut(scanner.Text(), "#")
		line := strings.TrimSpace(source)
		if line == "" {
			continue
		}
		left, right, ok := strings.Cut(line, " -> ")
		if !ok {
			return nil, fmt.Errorf("官方ProGuard映射第%d行无法解析", lines)
		}
		if strings.HasSuffix(right, ":") {
			name, mapped := strings.ReplaceAll(left, ".", "/"), strings.ReplaceAll(strings.TrimSuffix(right, ":"), ".", "/")
			if !mappingName(name) || !mappingName(mapped) || len(byName) >= 50000 {
				return nil, errors.New("官方映射类声明无效或过多")
			}
			if previous, ok := byName[name]; ok {
				if previous.mapped != mapped {
					return nil, errors.New("官方映射类重复冲突")
				}
				current = previous
				continue
			}
			current = &pgClass{name: name, mapped: mapped, fields: make(map[string]pgMember), methods: make(map[string]pgMember)}
			byName[name] = current
			continue
		}
		if current == nil || !mappingName(right) {
			return nil, errors.New("官方映射成员缺少类或有效名称")
		}
		members++
		if members > 750000 {
			return nil, errors.New("官方映射成员超过边界")
		}
		declaration := strings.TrimSpace(left)
		if strings.ContainsRune(declaration, '(') {
			for count := 0; count < 2; count++ {
				prefix, rest, ok := strings.Cut(declaration, ":")
				if !ok || !decimalMapping(prefix) {
					break
				}
				declaration = rest
			}
			result, method, ok := strings.Cut(declaration, " ")
			if !ok {
				return nil, errors.New("映射方法缺少返回类型")
			}
			open, close := strings.IndexByte(method, '('), strings.IndexByte(method, ')')
			if open < 1 || close < open {
				return nil, errors.New("映射方法参数无效")
			}
			name := strings.ReplaceAll(method[:open], ".", "/")
			if !mappingName(name) {
				return nil, errors.New("映射方法名称无效")
			}
			var descriptor strings.Builder
			descriptor.WriteByte('(')
			parameters := method[open+1 : close]
			if parameters != "" {
				for _, parameter := range strings.Split(parameters, ",") {
					desc, err := mappingType(strings.TrimSpace(parameter), false)
					if err != nil {
						return nil, err
					}
					descriptor.WriteString(desc)
				}
			}
			descriptor.WriteByte(')')
			desc, err := mappingType(result, true)
			if err != nil {
				return nil, err
			}
			descriptor.WriteString(desc)
			key := name + descriptor.String()
			value := pgMember{name, right, descriptor.String()}
			if old, ok := current.methods[key]; ok && old.mapped != right {
				return nil, errors.New("官方映射方法重名冲突")
			}
			current.methods[key] = value
		} else {
			fields := strings.Fields(declaration)
			if len(fields) != 2 || !mappingName(fields[1]) {
				return nil, errors.New("官方映射字段无效")
			}
			desc, err := mappingType(fields[0], false)
			if err != nil {
				return nil, err
			}
			name := strings.ReplaceAll(fields[1], ".", "/")
			if old, ok := current.fields[name]; ok && (old.mapped != right || old.desc != desc) {
				return nil, errors.New("官方映射字段重复冲突")
			}
			current.fields[name] = pgMember{name, right, desc}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(byName) == 0 {
		return nil, errors.New("官方映射为空")
	}
	classes := make([]*pgClass, 0, len(byName))
	for _, class := range byName {
		classes = append(classes, class)
	}
	slices.SortFunc(classes, func(a, b *pgClass) int { return javaLexical(a.name, b.name) })
	return classes, nil
}
func mappingName(value string) bool {
	return value != "" && len(value) <= 4096 && !strings.ContainsAny(value, string([]byte{32, 9, 13, 10})+";()[]:")
}
func decimalMapping(value string) bool {
	return value != "" && strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) < 0
}
func mappingType(value string, allowVoid bool) (string, error) {
	arrays := 0
	for strings.HasSuffix(value, "[]") {
		arrays++
		value = strings.TrimSuffix(value, "[]")
		if arrays > 255 {
			return "", errors.New("映射数组维度过多")
		}
	}
	primitive := map[string]string{"boolean": "Z", "byte": "B", "char": "C", "short": "S", "int": "I", "long": "J", "float": "F", "double": "D", "void": "V"}
	desc := primitive[value]
	if desc == "" {
		name := strings.ReplaceAll(value, ".", "/")
		if !mappingName(name) || strings.ContainsAny(name, "<>,") {
			return "", errors.New("映射类型无效")
		}
		desc = "L" + name + ";"
	}
	if desc == "V" && (!allowVoid || arrays != 0) {
		return "", errors.New("映射void类型位置无效")
	}
	return strings.Repeat("[", arrays) + desc, nil
}
func javaLexical(a, b string) int {
	if a == b {
		return 0
	}
	if strings.IndexFunc(a, func(r rune) bool { return r > 127 }) < 0 && strings.IndexFunc(b, func(r rune) bool { return r > 127 }) < 0 {
		return strings.Compare(a, b)
	}
	return slices.Compare(utf16.Encode([]rune(a)), utf16.Encode([]rune(b)))
}
func writeTSRG(ctx context.Context, classes []*pgClass, format string, mappedOrder bool) ([]byte, error) {
	names := make(map[string]string, len(classes))
	for _, class := range classes {
		names[class.name] = class.mapped
	}
	remapDescriptor := func(desc string) string {
		var out strings.Builder
		for len(desc) > 0 {
			index := strings.IndexByte(desc, 'L')
			if index < 0 {
				out.WriteString(desc)
				break
			}
			out.WriteString(desc[:index+1])
			desc = desc[index+1:]
			end := strings.IndexByte(desc, ';')
			if end < 0 {
				return ""
			}
			name := desc[:end]
			if mapped, ok := names[name]; ok {
				name = mapped
			}
			out.WriteString(name)
			out.WriteByte(';')
			desc = desc[end+1:]
		}
		return out.String()
	}
	var out bytes.Buffer
	line := func(text string) { out.WriteString(text); out.WriteByte(10) }
	if format == "TSRG2" {
		line("tsrg2 left right")
	}
	for _, class := range classes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line(class.name + " " + class.mapped)
		fields := make([]pgMember, 0, len(class.fields))
		for _, field := range class.fields {
			fields = append(fields, field)
		}
		slices.SortFunc(fields, func(a, b pgMember) int {
			if !mappedOrder {
				return javaLexical(a.name, b.name)
			}
			if cmp := javaLexical(a.mapped, b.mapped); cmp != 0 {
				return cmp
			}
			return javaLexical(remapDescriptor(a.desc), remapDescriptor(b.desc))
		})
		for _, field := range fields {
			extra := ""
			if format == "TSRG2" {
				extra = " " + field.desc
			}
			line(string(byte(9)) + field.name + extra + " " + field.mapped)
		}
		methods := make([]pgMember, 0, len(class.methods))
		for _, method := range class.methods {
			methods = append(methods, method)
		}
		slices.SortFunc(methods, func(a, b pgMember) int {
			if cmp := javaLexical(a.name, b.name); cmp != 0 {
				return cmp
			}
			return javaLexical(a.desc, b.desc)
		})
		for _, method := range methods {
			line(string(byte(9)) + method.name + " " + method.desc + " " + method.mapped)
		}
		if out.Len() > 128<<20 {
			return nil, errors.New("规范化映射超过128MiB")
		}
	}
	return out.Bytes(), ctx.Err()
}
