package loader

import (
	"testing"

	"mcl/internal/core/meta"
)

func TestFixedOfficialProcessorContracts(t *testing.T) {
	values := map[string]forgeValue{"INPUT": {"work/client.jar", true}, "OUTPUT": {"libraries/output.jar", true}, "MAPPING": {"libraries/mappings.tsrg", true}, "PATCH": {"work/embedded/client.lzma", true}, "LEFT": {"libraries/left.tsrg", true}, "RIGHT": {"libraries/right.tsrg", true}}
	for _, tc := range []struct {
		tool string
		args []string
	}{
		{"net.minecraftforge:ForgeAutoRenamingTool:1.0.6", []string{"--input", "{INPUT}", "--output", "{OUTPUT}", "--names", "{MAPPING}", "--ann-fix", "--ids-fix", "--src-fix", "--record-fix", "--strip-sigs"}},
		{"net.minecraftforge:binarypatcher:1.2.0", []string{"--clean", "{INPUT}", "--output", "{OUTPUT}", "--apply", "{PATCH}", "--data", "--unpatched"}},
		{"net.minecraftforge:installertools:1.4.0", []string{"--task", "MERGE_MAPPING", "--left", "{LEFT}", "--right", "{RIGHT}", "--output", "{OUTPUT}", "--classes", "--reverse-right"}},
	} {
		if _, err := prepareForgeProcessor(forgeProcessor{Jar: tc.tool, Args: tc.args}, values, t.TempDir()); err != nil {
			t.Fatal("fixed official option shape rejected", tc.tool, err)
		}
	}
}

func TestFixedProcessorRejectsImplicitWritesUnknownOptionsAndWrongTypes(t *testing.T) {
	cases := []struct {
		tool string
		args []forgeValue
	}{
		{"net.minecraftforge:ForgeAutoRenamingTool:1.0.6", []forgeValue{{text: "--input"}, {"work/client.jar", true}}},
		{"net.minecraftforge:ForgeAutoRenamingTool:1.0.6", []forgeValue{{text: "--input"}, {"work/client.jar", true}, {text: "--output"}, {"libraries/out.jar", true}, {text: "--log"}, {"work/secret.log", true}}},
		{"net.minecraftforge:ForgeAutoRenamingTool:1.0.6", []forgeValue{{text: "--cfg"}, {"work/options.txt", true}}},
		{"net.minecraftforge:binarypatcher:1.2.0", []forgeValue{{text: "--clean"}, {text: "not-a-fixed-path"}, {text: "--output"}, {"libraries/out.jar", true}, {text: "--apply"}, {"work/patch.lzma", true}}},
		{"net.minecraftforge:binarypatcher:1.2.0", []forgeValue{{text: "--clean"}, {"work/client.jar", true}, {text: "--output"}, {"libraries/out.jar", true}, {text: "--apply"}, {"work/patch.lzma", true}, {text: "--legacy"}}},
		{"net.minecraftforge:installertools:1.4.0", []forgeValue{{text: "--task"}, {text: "MERGE_MAPPING"}, {text: "--left"}, {"libraries/a", true}, {text: "--left"}, {"libraries/b", true}, {text: "--right"}, {"libraries/c", true}, {text: "--output"}, {"libraries/out.jar", true}}},
	}
	for _, tc := range cases {
		c, _ := meta.ParseCoordinate(tc.tool)
		if err := validateProcessorContract(c, tc.args, map[string]string{"libraries/out.jar": ""}); err == nil {
			t.Fatal("uncontrolled tool shape accepted", tc.tool)
		}
	}
}
