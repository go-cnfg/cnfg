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

func TestDir(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"10-base.conf":  `{"addr": ":1111", "timeout": "90s"}`,
		"20-local.conf": `{"addr": ":2222"}`,
		"notes.txt":     `{"addr": ":2222"}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	cfg, err := cnfg.Parse(defaults(), json.Dir[Config](dir))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Addr != ":2222" {
		t.Errorf("last file wins: got %q", cfg.Addr)
	}
	if cfg.Timeout != 90*time.Second {
		t.Errorf("first file: got %v", cfg.Timeout)
	}
}

func TestDirMissing(t *testing.T) {
	cfg, err := cnfg.Parse(defaults(), json.Dir[Config](filepath.Join(t.TempDir(), "conf.d")))
	if err != nil {
		t.Fatalf("missing dir should be a no op: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("defaults kept: got %q", cfg.Addr)
	}
}
