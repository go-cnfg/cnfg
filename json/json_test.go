package json_test

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-cnfg/cnfg"
	"github.com/go-cnfg/cnfg/json"
)

type Config struct {
	Addr    string
	Workers int
	Timeout time.Duration
	Server  struct {
		TLSCert string
	}
}

func defaults() Config { return Config{Addr: ":8080", Workers: 4, Timeout: time.Second} }

func writeFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestFile(t *testing.T) {
	cfg, err := cnfg.Parse(defaults(), json.File[Config](writeFile(t, "config.json", `{"addr": ":1111", "timeout": "90s", "server": {"tls-cert": "file.pem"}}`)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Addr != ":1111" {
		t.Errorf("addr: got %q", cfg.Addr)
	}
	if cfg.Timeout != 90*time.Second {
		t.Errorf("timeout: got %v", cfg.Timeout)
	}
	if cfg.Server.TLSCert != "file.pem" {
		t.Errorf("nested: got %q", cfg.Server.TLSCert)
	}
	if cfg.Workers != 4 {
		t.Errorf("untouched default: got %d", cfg.Workers)
	}
}

func TestFileFlag(t *testing.T) {
	path := writeFile(t, "config.json", `{"addr": ":1111", "timeout": "90s", "server": {"tls-cert": "file.pem"}}`)
	args := []string{"-config", path, "-workers", "9"}

	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.SetOutput(io.Discard)

	cfg, err := cnfg.Parse(defaults(),
		json.FileFlag[Config](set, "config", args),
		cnfg.FlagSet[Config](set, args),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Addr != ":1111" {
		t.Errorf("file from flag: got %q", cfg.Addr)
	}
	if cfg.Workers != 9 {
		t.Errorf("flag over file: got %d", cfg.Workers)
	}
}

func TestOptional(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")

	if _, err := cnfg.Parse(defaults(), cnfg.Optional(json.File[Config](missing))); err != nil {
		t.Errorf("optional file should be ignored: %v", err)
	}
	if _, err := cnfg.Parse(defaults(), json.File[Config](missing)); !errors.Is(err, cnfg.ErrReadFile) {
		t.Errorf("got %v, want ErrReadFile", err)
	}
}

func TestBrokenFile(t *testing.T) {
	_, err := cnfg.Parse(defaults(), json.File[Config](writeFile(t, "config.json", "{")))
	if !errors.Is(err, cnfg.ErrDecodeFile) {
		t.Errorf("got %v, want ErrDecodeFile", err)
	}
}
