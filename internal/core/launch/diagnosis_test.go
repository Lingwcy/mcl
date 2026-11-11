package launch

import (
	"strings"
	"testing"
)

func TestRuntimeDiagnosisDoesNotTreatAudioOrLastWarningAsSuccessOrCause(t *testing.T) {
	output := OutputSnapshot{Lines: []Line{{Text: "OpenAL initialized"}, {Text: "Sound engine started"}, {Text: "AL lib: (EE) alc_cleanup: 1 device not closed"}, {Text: "WARN mod configuration fallback"}}}
	report := DiagnoseRuntime(output, Exit{}, true, "pending", nil)
	if len(report.Findings) != 1 || report.Findings[0].Code != "insufficient-evidence" {
		t.Fatal("warning/sound guessed a failure cause")
	}
	text := report.Text()
	for _, want := range []string{"audio-initialization=log-observed", "window=pending", "main-menu=unconfirmed"} {
		if !strings.Contains(text, want) {
			t.Fatal("missing independent observation", want)
		}
	}
	if strings.Contains(text, "main-menu=confirmed") {
		t.Fatal("sound proved main menu")
	}
}

func TestRuntimeDiagnosisKeepsSpecificExceptionsAndStopOwnership(t *testing.T) {
	for _, tc := range []struct{ text, code string }{
		{"java.lang.UnsupportedClassVersionError: class version 65", "java-class-version"},
		{"java.lang.NoClassDefFoundError: org/lwjgl/LWJGLException", "class-loading"},
		{"java.lang.UnsatisfiedLinkError: wrong ELF class", "native-load"},
		{"GLFW error 65542: driver does not support OpenGL", "graphics-window"},
		{"Missing mandatory dependencies: example", "mod-dependency"},
		{"MixinApplyError: failed apply", "mod-transformation"},
		{"java.lang.OutOfMemoryError: Java heap space", "memory"},
		{"AuthenticationException: Invalid session", "authentication"},
		{"java.net.UnknownHostException: unavailable", "external-network"},
		{"A fatal error has been detected by the Java Runtime: SIGSEGV", "jvm-native-crash"},
	} {
		report := DiagnoseRuntime(OutputSnapshot{Lines: []Line{{Text: tc.text}, {Text: "WARN unrelated shutdown"}}}, Exit{Code: 1, Stopped: true}, false, "visible", nil)
		if len(report.Findings) != 1 || report.Findings[0].Code != tc.code {
			t.Fatalf("specific evidence %s misclassified", tc.code)
		}
		if !strings.Contains(report.Text(), "exit-owner=user-stop") {
			t.Fatal("manual stop attributed to an exception")
		}
	}
}

func TestDiagnosticTextNeverExportsSecretsURLsOrFullCommands(t *testing.T) {
	secret := "synthetic-session-secret-with-sensitive-content"
	scrub := func(s string) string { return strings.ReplaceAll(s, secret, "[REDACTED]") }
	for _, raw := range []string{
		"Authorization: Bearer another-private-value", "Cookie: key=private-value; other=second-secret",
		"access_token=private-value more fields", "password: private value with spaces",
		"refreshToken=private-value", "URL https://host.invalid/private?token=private-value#private",
		"value eyJheader12.payload12.signature12", "before " + secret + " after",
		"Command Line: java Main --accessToken private-value", "java_command: Main private-value",
		"unlabelled Main --accessToken private-value", "launching with arguments [private-value]",
	} {
		result := DiagnosticText(raw, scrub)
		for _, forbidden := range []string{"private-value", "second-secret", secret, "eyJheader12", "https://", "private value with spaces"} {
			if strings.Contains(result, forbidden) {
				t.Fatalf("sensitive diagnostic retained %q", forbidden)
			}
		}
	}
	// The sanitizer must not destroy the exception needed to understand a failure.
	if got := DiagnosticText("java.lang.NoClassDefFoundError: org/example/Class", nil); !strings.Contains(got, "org/example/Class") {
		t.Fatal("non-secret class evidence lost")
	}
}

func TestDiagnosisSourcesAreBoundedAndSanitizedCopies(t *testing.T) {
	lines := []string{"Authorization: Bearer private-value", "https://host.invalid/secret", "java.lang.NoClassDefFoundError: fixed.Class"}
	input := []DiagnosticLog{{Kind: "latest", Association: "changed-in-session", Lines: lines}}
	report := DiagnoseRuntime(OutputSnapshot{}, Exit{}, true, "untrusted-window-string", input)
	if strings.Contains(report.Text(), "private-value") || strings.Contains(report.Text(), "https://") || strings.Contains(report.Text(), "untrusted-window-string") {
		t.Fatal("report exported raw input")
	}
	report.Sources[0].Lines[0] = "changed"
	if input[0].Lines[0] == "changed" {
		t.Fatal("report aliases source lines")
	}
	if len(DiagnoseRuntime(OutputSnapshot{}, Exit{}, false, "unsupported", []DiagnosticLog{{Kind: "/outside/secret", Lines: lines}}).Sources) != 0 {
		t.Fatal("arbitrary log source admitted")
	}
}
