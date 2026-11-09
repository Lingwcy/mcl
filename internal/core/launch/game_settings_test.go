package launch

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mcl/internal/core/meta"
)

func TestGameSettingsValidationAndServerAddresses(t *testing.T) {
	for _, settings := range []GameSettings{
		{Width: 319, Height: 480}, {Width: 854}, {Height: 480},
		{Width: 854, Height: 16385}, {CustomInfo: strings.Repeat("猫", 121)},
		{CustomInfo: "--accessToken=other"}, {CustomInfo: "line\nfeed"},
		{AutoJoinServer: true}, {ServerAddress: "https://example.com"},
		{ServerAddress: "example.com:0"}, {ServerAddress: "example.com:65536"},
		{ServerAddress: "user@example.com"}, {ServerAddress: "host/other"},
		{ServerAddress: "example.com\n"}, {ServerAddress: "[not-ip]:25565"},
	} {
		if settings.Validate() == nil {
			t.Errorf("accepted invalid game settings: %+v", settings)
		}
	}
	for _, tc := range []struct {
		address, host string
		port          int
	}{
		{"mc.example.com", "mc.example.com", 25565},
		{"localhost:25566", "localhost", 25566},
		{"127.0.0.1:1", "127.0.0.1", 1},
		{"::1", "::1", 25565}, {"[::1]", "::1", 25565},
		{"[2001:db8::1]:65535", "2001:db8::1", 65535},
		{"例子.测试", "xn--fsqu00a.xn--0zwm56d", 25565},
	} {
		host, port, err := (GameSettings{ServerAddress: tc.address}).Server()
		if err != nil || host != tc.host || port != tc.port {
			t.Errorf("address %q: host=%q port=%d error=%v", tc.address, host, port, err)
		}
	}
}

func TestConfiguredGameArgumentsKeepIdentityAndPhysicalVersion(t *testing.T) {
	for _, id := range []string{"1.20.4", "1.21.1"} {
		t.Run(id, func(t *testing.T) {
			o := options(t, id)
			o.Game = GameSettings{Width: 1280, Height: 720, Fullscreen: true, CustomInfo: "我的 mcl", AutoJoinServer: true, ServerAddress: "[::1]:25566"}
			o.Platform.Features = map[string]bool{"is_demo_user": true, "is_quick_play_realms": true}
			identity := session(t)
			plan, err := Build(o, identity)
			if err != nil {
				t.Fatal(err)
			}
			args := plan.Preview().Arguments
			for flag, want := range map[string]string{"--width": "1280", "--height": "720", "--version": "我的 mcl", "--versionType": "我的 mcl", "--quickPlayMultiplayer": "[::1]:25566", "--username": "Player"} {
				index := slices.Index(args, flag)
				if index < 0 || index+1 >= len(args) || args[index+1] != want {
					t.Errorf("%s did not receive %q: %v", flag, want, args)
				}
			}
			if !slices.Contains(args, "--fullscreen") || slices.Contains(args, "--server") || slices.Contains(args, "--demo") || slices.Contains(args, "--quickPlayRealms") {
				t.Fatal("unexpected structured feature selection")
			}
			cp := slices.Index(args, "-cp")
			if cp < 0 || !strings.Contains(args[cp+1], id+".jar") || strings.Contains(args[cp+1], "我的 mcl") {
				t.Fatal("custom information changed the physical client path")
			}
			if strings.Contains(strings.Join(args, " "), identity.token) {
				t.Fatal("preview leaked the session token")
			}
		})
	}
}

func TestLegacyMetadataServerFallbackAndLiteralInfo(t *testing.T) {
	o := options(t, "1.20.4")
	o.Version.Arguments.Game = slices.DeleteFunc(o.Version.Arguments.Game, func(argument meta.Argument) bool {
		return slices.Contains(argument.Values, "--quickPlayMultiplayer")
	})
	o.Game = GameSettings{AutoJoinServer: true, ServerAddress: "example.com:25566", CustomInfo: "${auth_access_token}"}
	plan, err := Build(o, session(t))
	if err != nil {
		t.Fatal(err)
	}
	args := plan.Preview().Arguments
	for flag, want := range map[string]string{"--server": "example.com", "--port": "25566", "--version": "${auth_access_token}"} {
		index := slices.Index(args, flag)
		if index < 0 || index+1 >= len(args) || args[index+1] != want {
			t.Errorf("unexpected %s replacement", flag)
		}
	}
}

func TestExplicitGameDirectoryDoesNotMoveSharedRuntimePaths(t *testing.T) {
	o := options(t, "1.20.4")
	o.GameDirectory = filepath.Join(t.TempDir(), "外部游戏", "game")
	preview, err := PreviewOptions(o)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Directory != o.GameDirectory {
		t.Fatal("working directory ignored the instance location")
	}
	args := preview.Arguments
	index := slices.Index(args, "--gameDir")
	if index < 0 || index+1 >= len(args) || args[index+1] != o.GameDirectory {
		t.Fatal("official game directory argument ignored the location")
	}
	cp := slices.Index(args, "-cp")
	if cp < 0 || !strings.Contains(args[cp+1], o.Root) || strings.Contains(args[cp+1], o.GameDirectory) {
		t.Fatal("location choice moved shared client or libraries")
	}
	o.GameDirectory = "relative/game"
	if _, err := PreviewOptions(o); err == nil {
		t.Fatal("accepted a relative working directory")
	}
}

func TestProcessPriorityIsFrozenInPlanAndNeverBecomesAnArgument(t *testing.T) {
	o := options(t, "1.20.4")
	for _, priority := range []ProcessPriority{PriorityNormal, PriorityLow, PriorityBelowNormal, PriorityAboveNormal, PriorityHigh} {
		o.Game.Priority = priority
		o.Game.ShowLogs = true
		plan, err := Build(o, session(t))
		if err != nil {
			t.Fatal(err)
		}
		o.Game.Priority = PriorityNormal
		if plan.Priority() != priority || plan.Preview().Priority != priority {
			t.Fatal("process priority was not fixed to this launch")
		}
		for _, arg := range plan.Preview().Arguments {
			if strings.Contains(arg, "process_priority") || strings.Contains(arg, "show_logs") {
				t.Fatal("launcher-only preferences entered the game command")
			}
		}
	}
	o.Game.Priority = ProcessPriority("realtime")
	if _, err := Build(o, session(t)); err == nil {
		t.Fatal("unvalidated priority reached a launch plan")
	}
}
