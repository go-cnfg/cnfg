package cnfg_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-cnfg/cnfg"
)

func writeDir(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return dir
}

func TestGlob(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"20-app.conf":    `{"addr": ":2222", "worker_count": 2}`,
		"10-base.conf":   `{"addr": ":1111", "ratio": 1.5}`,
		"99-local.conf":  `{"addr": ":9999"}`,
		"disabled.conf~": `{"addr": ":6666"}`,
		"notes.txt":      `{"addr": ":7777"}`,
	})

	cfg, err := cnfg.Parse(defaults(), cnfg.Glob(json.Unmarshal, filepath.Join(dir, "*.conf")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "last file wins", ":9999", cfg.Addr)
	assertEqual(t, "earlier file", 2, cfg.Workers)
	assertEqual(t, "first file", 1.5, cfg.Ratio)
	assertEqual(t, "untouched default", time.Second, cfg.Timeout)
}

func TestGlobNoMatches(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "conf.d", "*.conf")

	cfg, err := cnfg.Parse(defaults(), cnfg.Glob(json.Unmarshal, missing))
	if err != nil {
		t.Fatalf("a directory that is not there should be a no op: %v", err)
	}
	assertEqual(t, "defaults kept", ":8080", cfg.Addr)
}

func TestGlobSkipsDirs(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"10-base.conf":      `{"addr": ":1111"}`,
		"20-sub.conf/x.txt": "",
	})

	cfg, err := cnfg.Parse(defaults(), cnfg.Glob(json.Unmarshal, filepath.Join(dir, "*.conf")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "file read", ":1111", cfg.Addr)
}

func TestGlobBrokenFile(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"10-base.conf": `{"addr": ":1111"}`,
		"20-bad.conf":  "{",
	})

	_, err := cnfg.Parse(defaults(), cnfg.Glob(json.Unmarshal, filepath.Join(dir, "*.conf")))
	if !errors.Is(err, cnfg.ErrDecodeFile) {
		t.Fatalf("got %v, want ErrDecodeFile", err)
	}
	if !strings.Contains(err.Error(), "20-bad.conf") {
		t.Errorf("error should name the file: %v", err)
	}
}

func TestGlobBadPattern(t *testing.T) {
	_, err := cnfg.Parse(defaults(), cnfg.Glob(json.Unmarshal, "["))
	if !errors.Is(err, cnfg.ErrReadFile) {
		t.Errorf("got %v, want ErrReadFile", err)
	}
}

func TestDropInDirectory(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"10-base.conf":  `{"addr": ":1111", "timeout": "90s"}`,
		"20-local.conf": `{"addr": ":2222"}`,
		"notes.txt":     `{"addr": ":7777"}`,
	})

	cfg, err := cnfg.Parse(defaults(), cnfg.Glob(json.Unmarshal, filepath.Join(dir, "*.conf")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "last file wins", ":2222", cfg.Addr)
	assertEqual(t, "earlier file", 90*time.Second, cfg.Timeout)
	assertEqual(t, "other extension skipped", 4, cfg.Workers)
}

func TestDropInDirectoryMissing(t *testing.T) {
	cfg, err := cnfg.Parse(defaults(), cnfg.Glob(json.Unmarshal, filepath.Join(t.TempDir(), "conf.d", "*.conf")))
	if err != nil {
		t.Fatalf("missing dir should be a no op: %v", err)
	}
	assertEqual(t, "defaults kept", ":8080", cfg.Addr)
}
