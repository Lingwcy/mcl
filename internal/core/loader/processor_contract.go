package loader

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"mcl/internal/core/meta"
)

type processorOption struct {
	role               string
	required, repeated bool
}

// Exact version contracts come from fixed official sources, not the names of
// flags found in another tool. Unsupported versions retain the existing bounded
// adapter; they do not inherit an unproven Java/JDK or option-version claim.
func validateProcessorContract(c meta.Coordinate, values []forgeValue, outputs map[string]string) error {
	if c.Group != "net.minecraftforge" || c.Classifier != "" {
		return nil
	}
	options := make(map[string]processorOption)
	add := func(role string, required, repeated bool, names ...string) {
		for _, name := range names {
			options[name] = processorOption{role, required, repeated}
		}
	}
	switch {
	case c.Artifact == "ForgeAutoRenamingTool" && c.Version == "1.0.6":
		add("input", true, false, "--input")
		add("output", true, false, "--output") // official default overwrites input; mcl requires an explicit private output
		add("input", false, false, "--names", "--map", "--ff-line-numbers")
		add("input", false, true, "--lib", "--e")
		add("flag", false, false, "--ann-fix", "--record-fix", "--ids-fix", "--src-fix", "--strip-sigs", "--reverse", "--disable-abstract-param")
		add("threads", false, false, "--threads")
	case c.Artifact == "binarypatcher" && c.Version == "1.2.0":
		add("input", true, false, "--clean", "--apply")
		add("output", true, false, "--output")
		add("flag", false, false, "--data", "--unpatched")
		add("literal", false, false, "--prefix")
	case c.Artifact == "installertools" && c.Version == "1.4.0":
		task := ""
		for i := 0; i+1 < len(values); i++ {
			if values[i].text == "--task" && !values[i+1].file {
				task = values[i+1].text
			}
		}
		switch task {
		case "MCP_DATA", "EXTRACT_FILES", "CREATE_DIR", "CREATE_PARENTS", "BUNDLER_EXTRACT", "DOWNLOAD_MOJMAPS":
			return nil // stricter dedicated native/mapping adapters validate these
		case "MERGE_MAPPING", "CHAIN_MAPPING":
			add("literal", true, false, "--task")
			add("input", true, false, "--left", "--right")
			add("output", true, false, "--output")
			add("literal", false, false, "--left-names", "--right-names")
			add("flag", false, false, "--reverse-left", "--reverse-right", "--classes", "--fields", "--methods", "--params")
		case "EXTRACT_INHERITANCE":
			add("literal", true, false, "--task")
			add("input", true, false, "--input")
			add("input", false, true, "--lib")
			add("output", true, false, "--output")
			add("flag", false, false, "--annotations")
		case "SRG_TO_MCP":
			add("literal", true, false, "--task")
			add("input", true, false, "--mcp", "--input")
			add("output", true, false, "--output")
			add("flag", false, false, "--strip-signatures")
		default:
			return errors.New("此固定安装工具任务尚无选项契约")
		}
	default:
		return nil
	}
	seen := make(map[string]int)
	outputPaths := make(map[string]bool)
	for i := 0; i < len(values); i++ {
		value := values[i]
		option, ok := options[value.text]
		if value.file || !ok {
			return errors.New("固定安装工具包含未知选项或不支持的可选值形态")
		}
		seen[value.text]++
		if seen[value.text] > 1 && !option.repeated {
			return fmt.Errorf("固定安装工具重复选项 %s", value.text)
		}
		if option.role == "flag" {
			continue
		}
		if i+1 >= len(values) {
			return fmt.Errorf("固定安装工具选项 %s 缺少值", value.text)
		}
		i++
		operand := values[i]
		switch option.role {
		case "input":
			if !operand.file || !ownedProcessorPath(operand.text) || operand.text == "." || operand.text == "libraries" || operand.text == "work" {
				return errors.New("固定安装工具输入没有绑定私有文件或目录")
			}
		case "output":
			if !forgeOutput(operand) {
				return errors.New("固定安装工具输出不在私有运行库中")
			}
			if _, ok := outputs[operand.text]; !ok {
				return errors.New("固定安装工具输出不在产物计划内")
			}
			outputPaths[operand.text] = true
		case "literal":
			if operand.file || !safeForgeLiteral(operand.text) || operand.text == "" || strings.HasPrefix(operand.text, "-") {
				return errors.New("固定安装工具标量值无效")
			}
		case "threads":
			n, err := strconv.Atoi(operand.text)
			if operand.file || err != nil || n < 1 || n > 256 {
				return errors.New("固定重命名工具线程数应为1–256")
			}
		}
	}
	for name, option := range options {
		if option.required && seen[name] == 0 {
			return fmt.Errorf("固定安装工具缺少选项 %s", name)
		}
	}
	for name := range outputs {
		if !outputPaths[name] {
			return errors.New("固定安装工具声明了未由输出选项绑定的产物")
		}
	}
	for _, aliases := range [][]string{{"--names", "--map"}, {"--lib", "--e"}} {
		if !slices.ContainsFunc(aliases, func(name string) bool { return seen[name] > 0 }) {
			continue
		}
		if seen[aliases[0]] > 0 && seen[aliases[1]] > 0 {
			return errors.New("固定安装工具混用了同一选项的别名")
		}
	}
	return nil
}
