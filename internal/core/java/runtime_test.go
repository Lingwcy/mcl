package java

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"mcl/internal/core/meta"
)

func TestMain(m *testing.M) {
	switch os.Getenv("MCL_TEST_JAVA_MODE") {
	case "properties":
		fmt.Fprintln(os.Stderr, "    java.version = 17.0.20.1\n    os.arch = aarch64\n    java.vendor = Test JVM")
		os.Exit(0)
	case "hang":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "overflow":
		for range 20000 {
			fmt.Fprintln(os.Stderr, "a deliberately excessive diagnostic line")
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestParsePropertiesAndRequirements(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		version string
		arch    string
		major   int
		ea      bool
	}{
		{"1.8.0_442", "x86_64", 8, false},
		{"17.0.20.1", "aarch64", 17, false},
		{"21.0.8+9-LTS", "amd64", 21, false},
		{"27-ea", "aarch64", 27, true},
	} {
		got, err := ParseProperties(fmt.Sprintf("    java.version = %s\r\n    os.arch = %s\r\n", test.version, test.arch))
		if err != nil || got.Major != test.major || got.EarlyAccess != test.ea {
			t.Fatalf("parse %s: %+v, %v", test.version, got, err)
		}
	}
	runtime := Runtime{Version: "17.0.20.1", Major: 17, Arch: "arm64"}
	if err := runtime.Check(17, meta.Platform{Arch: "aarch64"}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Check(21, meta.Platform{Arch: "arm64"}); err == nil {
		t.Fatal("Java 17 cannot satisfy a Java 21 requirement")
	}
	if err := runtime.Check(17, meta.Platform{Arch: "amd64"}); err == nil {
		t.Fatal("architecture mismatch must be explicit")
	}
	runtime.EarlyAccess = true
	if err := runtime.Check(17, meta.Platform{Arch: "arm64"}); err == nil {
		t.Fatal("early-access JVM cannot silently count as supported")
	}
	if _, err := ParseProperties("java.version = unknown\nos.arch = mystery"); err == nil {
		t.Fatal("unknown diagnostic output must not be guessed")
	}
}

func TestProbeProcessBoundaries(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MCL_TEST_JAVA_MODE", "properties")
	runtime, err := Probe(t.Context(), executable)
	if err != nil || runtime.Major != 17 || runtime.Arch != "arm64" || runtime.Executable == "" {
		t.Fatalf("probe executable: %+v, %v", runtime, err)
	}
	t.Setenv("MCL_TEST_JAVA_MODE", "overflow")
	if _, err := Probe(t.Context(), executable); err == nil {
		t.Fatal("excessive diagnostics should fail within the bounded buffer")
	}
	t.Setenv("MCL_TEST_JAVA_MODE", "hang")
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Millisecond)
	defer cancel()
	if _, err := Probe(ctx, executable); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("probe must preserve cancellation: %v", err)
	}
}
