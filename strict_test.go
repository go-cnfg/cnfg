package cnfg_test

import (
	"encoding/json"
	"errors"
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-cnfg/cnfg"
)

type Strictly struct {
	Addr   string
	Server struct {
		TLSCert string
	}
	Items    []Item
	Metadata map[string]string
	Free     any
	Secret   string `cnfg:"-"`
}

func strictFile(t *testing.T, content string) cnfg.Source {
	t.Helper()

	return cnfg.FileStrict(json.Unmarshal, writeFile(t, "strict.json", content))
}

func TestStrictFileAccepts(t *testing.T) {
	cfg, err := cnfg.Parse(Strictly{}, strictFile(t, `{
		"addr": ":1111",
		"server": {"TLSCert": "a.pem"},
		"items": [{"name": "a", "port": 1}],
		"metadata": {"env": "test", "anything": "goes"},
		"free": {"whatever": {"nested": true}}
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "addr", ":1111", cfg.Addr)
	assertEqual(t, "map key is a value", "test", cfg.Metadata["env"])
}

func TestStrictFileRejects(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "top level", content: `{"addr": ":1", "worker_cont": 8}`, want: "worker_cont"},
		{name: "nested", content: `{"server": {"tls-cret": "a.pem"}}`, want: "server.tls-cret"},
		{name: "slice element", content: `{"items": [{"name": "a"}, {"prt": 2}]}`, want: "items[1].prt"},
		{name: "skipped field", content: `{"secret": "x"}`, want: "secret"},
		{name: "first of several", content: `{"one": 1, "two": 2}`, want: "one"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := cnfg.Parse(Strictly{}, strictFile(t, tc.content))
			if !errors.Is(err, cnfg.ErrUnknownField) {
				t.Fatalf("got %v, want ErrUnknownField", err)
			}
			if !strings.HasSuffix(err.Error(), tc.want) {
				t.Errorf("error should name %q: %v", tc.want, err)
			}
		})
	}
}

func TestStrictFileIsOptIn(t *testing.T) {
	file := writeFile(t, "lenient.json", `{"addr": ":1", "worker_cont": 8}`)

	if _, err := cnfg.Parse(Strictly{}, cnfg.File(json.Unmarshal, file)); err != nil {
		t.Errorf("extra keys are fine outside FileStrict: %v", err)
	}
}

func TestStrictFileFromEnv(t *testing.T) {
	t.Setenv("APP_CONFIG", writeFile(t, "env.json", `{"addr": ":1", "adrr": ":2"}`))

	_, err := cnfg.Parse(Strictly{}, cnfg.FileFromEnvStrict(json.Unmarshal, "APP_CONFIG"))
	if !errors.Is(err, cnfg.ErrUnknownField) {
		t.Fatalf("got %v, want ErrUnknownField", err)
	}
	if !strings.HasSuffix(err.Error(), "adrr") {
		t.Errorf("error should name the key: %v", err)
	}
}

func TestStrictEnv(t *testing.T) {
	ok := cnfg.EnvFromStrict("APP", []string{"APP_ADDR=:1", "PATH=/bin", "OTHER_THING=x"})
	if _, err := cnfg.Parse(Strictly{}, ok); err != nil {
		t.Errorf("variables without the prefix are not ours: %v", err)
	}

	bad := cnfg.EnvFromStrict("APP", []string{"APP_ADDR=:1", "APP_ADRR=:2"})
	_, err := cnfg.Parse(Strictly{}, bad)
	if !errors.Is(err, cnfg.ErrUnknownField) {
		t.Fatalf("got %v, want ErrUnknownField", err)
	}
	if !strings.HasSuffix(err.Error(), "APP_ADRR") {
		t.Errorf("error should name the variable: %v", err)
	}
}

func TestStrictEnvFromProcess(t *testing.T) {
	t.Setenv("APP_ADDR", ":1")
	t.Setenv("APP_ADRR", ":2")

	_, err := cnfg.Parse(Strictly{}, cnfg.EnvStrict("APP"))
	if !errors.Is(err, cnfg.ErrUnknownField) {
		t.Fatalf("got %v, want ErrUnknownField", err)
	}
	if !strings.HasSuffix(err.Error(), "APP_ADRR") {
		t.Errorf("error should name the variable: %v", err)
	}
}

func TestStrictEnvNeedsPrefix(t *testing.T) {
	_, err := cnfg.Parse(Strictly{}, cnfg.EnvFromStrict("", []string{"ADDR=:1"}))
	if !errors.Is(err, cnfg.ErrNoPrefix) {
		t.Errorf("got %v, want ErrNoPrefix", err)
	}
}

func TestStrictGlob(t *testing.T) {
	dir := writeDir(t, map[string]string{"10-base.conf": `{"addr": ":1"}`, "20-typo.conf": `{"adrr": ":2"}`})

	_, err := cnfg.Parse(Strictly{}, cnfg.GlobStrict(json.Unmarshal, filepath.Join(dir, "*.conf")))
	if !errors.Is(err, cnfg.ErrUnknownField) {
		t.Fatalf("got %v, want ErrUnknownField", err)
	}
	if !strings.Contains(err.Error(), "20-typo.conf") {
		t.Errorf("error should name the file: %v", err)
	}
}

type StrictEmbedded struct {
	Common

	Extra  *Nested
	Addr   string
	Server *Server
}

type Common struct {
	LogLevel string
}

func TestStrictEmbeddedAndPointers(t *testing.T) {
	file := writeFile(t, "embedded.json", `{"log-level": "warn", "addr": ":1", "server": {"TLSCert": "a.pem"}, "extra": {"max_conns": 2}}`)

	if _, err := cnfg.Parse(StrictEmbedded{}, cnfg.FileStrict(json.Unmarshal, file)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	typo := writeFile(t, "typo.json", `{"log-lvl": "warn"}`)
	_, err := cnfg.Parse(StrictEmbedded{}, cnfg.FileStrict(json.Unmarshal, typo))
	if !errors.Is(err, cnfg.ErrUnknownField) {
		t.Fatalf("got %v, want ErrUnknownField", err)
	}
}

func TestStrictFileFromFlag(t *testing.T) {
	file := writeFile(t, "flagged.json", `{"addr": ":1", "adrr": ":2"}`)
	set := flag.NewFlagSet("app", flag.ContinueOnError)

	_, err := cnfg.Parse(Strictly{}, cnfg.FileFromFlagStrict(json.Unmarshal, set, "config", []string{"-config", file}))
	if !errors.Is(err, cnfg.ErrUnknownField) {
		t.Fatalf("got %v, want ErrUnknownField", err)
	}
	if !strings.HasSuffix(err.Error(), "adrr") {
		t.Errorf("error should name the key: %v", err)
	}
}
