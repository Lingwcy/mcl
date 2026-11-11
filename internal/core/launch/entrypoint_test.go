package launch

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func entrypointArgument(value string) meta.Argument { return meta.Argument{Values: []string{value}} }

func TestSelectedGameEntrypointConflicts(t *testing.T) {
	for _, tc := range []struct {
		main string
		args []string
	}{
		{"Main", []string{"--username", "${auth_player_name}", "--username=other"}},
		{"Main", []string{"--accessToken", "${auth_access_token}", "--accessToken", "private-value"}},
		{"Main", []string{"--session", "${auth_session}", "--uuid", "${auth_uuid}"}},
		{"Main", []string{"--launchTarget", "forge_client", "--launchTarget=other"}},
		{"Main", []string{"--launchTarget"}},
		{"Main", []string{"--launchTarget", "--username", "value"}},
		{"Main", []string{"--gameDir="}},
		{"Main", []string{"--tweakClass", "example.Tweaker"}},
		{"net.minecraft.launchwrapper.Launch", nil},
		{"net.minecraft.launchwrapper.Launch", []string{"--tweakClass", "example.Tweaker", "--tweakClass=example.Tweaker"}},
		{"net.minecraft.launchwrapper.Launch", []string{"--tweakClass", "${external_tweaker}"}},
		{"net.minecraft.launchwrapper.Launch", []string{"--tweakClass", "example.Tweaker", "--launchTarget", "forge_client"}},
	} {
		if err := validateEntrypointTemplates(tc.main, tc.args); err == nil || strings.Contains(err.Error(), "private-value") {
			t.Fatalf("ambiguous template accepted or raw values disclosed for %s", tc.main)
		}
	}
	// Do not deduplicate different declared tweakers or reorder their options.
	// The existing legacy Forge adapter independently restricts its one tweaker.
	args := []string{"--tweakClass", "first.Tweaker", "--tweakClass=second.Tweaker"}
	before := slices.Clone(args)
	if err := validateEntrypointTemplates("net.minecraft.launchwrapper.Launch", args); err != nil || !slices.Equal(args, before) {
		t.Fatal("changed distinct tweaker order", err)
	}
}

func TestBuildValidatesSelectedTemplatesBeforeIdentitySubstitution(t *testing.T) {
	o := options(t, "1.20.4")
	// A duplicate is inactive on Linux and active on Windows; rules, not raw
	// declaration count, decide whether the merged template is contradictory.
	duplicate := meta.Argument{Values: []string{"--username", "other"}}
	if err := json.Unmarshal([]byte("[{\"action\":\"allow\",\"os\":{\"name\":\"windows\"}}]"), &duplicate.Rules); err != nil {
		t.Fatal(err)
	}
	o.Version.Arguments.Game = append(o.Version.Arguments.Game, duplicate)
	o.Platform = meta.Platform{OS: "linux", Arch: "amd64"}
	o.Java.Arch = "amd64"
	s := session(t)
	s.Name = "--tweakClass"
	p, err := Build(o, s)
	if err != nil {
		t.Fatal("account data reinterpreted as an option", err)
	}
	if !slices.Contains(p.arguments, s.Name) {
		t.Fatal("account value changed")
	}
	o.Platform.OS = "windows"
	if _, err := Build(o, s); err == nil {
		t.Fatal("active duplicate template accepted")
	}
}

func TestJVMEntryMustRemainTheFixedMainClass(t *testing.T) {
	for _, args := range [][]string{
		{"-jar", "replacement.jar"}, {"--module=other/Main"}, {"-m", "other/Main"},
		{"--source", "21", "Source.java"}, {"--dry-run"}, {"--"}, {"ExtraMain"}, {"-h"}, {"--help-extra"}, {"-fullversion"}, {"--full-version"},
		{"--add-opens"}, {"--module-path", "-bad-value"}, {"@extra.args"},
	} {
		o := options(t, "1.20.4")
		for _, arg := range args {
			o.Version.Arguments.JVM = append(o.Version.Arguments.JVM, entrypointArgument(arg))
		}
		if _, err := Build(o, session(t)); err == nil {
			t.Fatalf("JVM entry override accepted: %q", args[0])
		}
	}
	o := options(t, "1.20.4")
	// A legal JVM operand position equal to the class text must not truncate
	// JVMArgumentFile or cause ProcessCommand to use a different boundary.
	o.Version.Arguments.JVM = append(o.Version.Arguments.JVM, entrypointArgument("--add-modules"), entrypointArgument(o.Version.MainClass), entrypointArgument("-Dafter=operand"))
	p, err := Build(o, session(t))
	if err != nil {
		t.Fatal(err)
	}
	data, err := p.JVMArgumentFile()
	if err != nil || !strings.Contains(string(data), "-Dafter=operand") {
		t.Fatal("JVM operand was mistaken for actual main", err)
	}
	args, err := p.ProcessCommand("/private/runtime/jvm.args")
	if err != nil || args[1] != o.Version.MainClass || slices.Contains(args, "-Dafter=operand") {
		t.Fatal("wrong direct main boundary", err)
	}
}
