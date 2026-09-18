package cnfg_test

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/go-cnfg/cnfg"
)

type mustConfig struct {
	Addr    string `cnfg:",require"           usage:"address to listen on"`
	Workers int    `usage:"number of workers"`
}

func TestMustParse(t *testing.T) {
	code, out := runMustParse(t, func() {
		cfg := cnfg.MustParse(mustConfig{Addr: ":8080"}, cnfg.FlagSet(mustSet(), []string{"-workers", "4"}))
		assertEqual(t, "workers", 4, cfg.Workers)
	})

	if code != nil {
		t.Errorf("got exit %d, want no exit", *code)
	}
	if out != "" {
		t.Errorf("got stderr %q, want nothing", out)
	}
}

func TestMustParseHelp(t *testing.T) {
	code, out := runMustParse(t, func() {
		cnfg.MustParse(mustConfig{Addr: ":8080"}, cnfg.FlagSet(mustSet(), []string{"-h"}))
	})

	if code == nil || *code != 0 {
		t.Errorf("got exit %v, want 0", code)
	}
	if !strings.Contains(out, "-addr string") {
		t.Errorf("stderr should hold the usage output: %q", out)
	}
}

func TestMustParseFlagError(t *testing.T) {
	code, out := runMustParse(t, func() {
		cnfg.MustParse(mustConfig{}, cnfg.FlagSet(mustSet(), []string{"-nope"}))
	})

	if code == nil || *code != 1 {
		t.Errorf("got exit %v, want 1", code)
	}

	const want = "flag provided but not defined: -nope"
	if n := strings.Count(out, want); n != 1 {
		t.Errorf("the flag set reports this once, got it %d times: %q", n, out)
	}
}

func TestMustParseError(t *testing.T) {
	code, out := runMustParse(t, func() {
		cnfg.MustParse(mustConfig{}, cnfg.Require())
	})

	if code == nil || *code != 1 {
		t.Errorf("got exit %v, want 1", code)
	}
	if !strings.Contains(out, "required field not set: addr") {
		t.Errorf("stderr should name the field: %q", out)
	}
}

func TestMustParseExitOnError(t *testing.T) {
	set := flag.NewFlagSet("app", flag.ExitOnError)

	code, out := runMustParse(t, func() {
		cnfg.MustParse(mustConfig{}, cnfg.FlagSet(set, []string{"-nope"}))
	})

	if code == nil || *code != 1 {
		t.Errorf("got exit %v, want 1", code)
	}
	if !strings.Contains(out, "flag provided but not defined: -nope") {
		t.Errorf("stderr should name the flag: %q", out)
	}
	if set.ErrorHandling() != flag.ContinueOnError {
		t.Errorf("got %v, want ContinueOnError", set.ErrorHandling())
	}
	if set.Name() != "app" {
		t.Errorf("the set should keep its name, got %q", set.Name())
	}
}

func TestMustParsePanicOnError(t *testing.T) {
	set := flag.NewFlagSet("app", flag.PanicOnError)

	code, out := runMustParse(t, func() {
		cnfg.MustParse(mustConfig{Addr: ":8080"}, cnfg.FlagSet(set, []string{"-h"}))
	})

	if code == nil || *code != 0 {
		t.Errorf("got exit %v, want 0", code)
	}
	if !strings.Contains(out, "-addr string") {
		t.Errorf("stderr should hold the usage output: %q", out)
	}
}

func mustSet() *flag.FlagSet {
	return flag.NewFlagSet("app", flag.ContinueOnError)
}

// runMustParse calls fn with exit and stderr captured, and gives the status it exited
// with, nil when it did not, together with what it wrote.
func runMustParse(t *testing.T, fn func()) (*int, string) {
	t.Helper()

	var code *int
	restore := cnfg.SetExit(func(c int) { code = &c })
	defer restore()

	f, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	old := os.Stderr
	os.Stderr = f
	defer func() { os.Stderr = old }()

	fn()

	if err := f.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	out, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("failed to read temp file: %v", err)
	}
	return code, string(out)
}
