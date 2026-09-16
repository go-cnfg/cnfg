package cnfg_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
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

func strictFile(t *testing.T, content string) cnfg.Parser[Strictly] {
	t.Helper()

	return cnfg.Decode[Strictly](json.Unmarshal, writeFile(t, "strict.json", content), cnfg.Strict)
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

	if _, err := cnfg.Parse(Strictly{}, cnfg.Decode[Strictly](json.Unmarshal, file)); err != nil {
		t.Errorf("extra keys are fine without the option: %v", err)
	}
}

func TestStrictEnv(t *testing.T) {
	ok := cnfg.EnvFrom[Strictly]("APP", []string{"APP_ADDR=:1", "PATH=/bin", "OTHER_THING=x"}, cnfg.Strict)
	if _, err := cnfg.Parse(Strictly{}, ok); err != nil {
		t.Errorf("variables without the prefix are not ours: %v", err)
	}

	bad := cnfg.EnvFrom[Strictly]("APP", []string{"APP_ADDR=:1", "APP_ADRR=:2"}, cnfg.Strict)
	_, err := cnfg.Parse(Strictly{}, bad)
	if !errors.Is(err, cnfg.ErrUnknownField) {
		t.Fatalf("got %v, want ErrUnknownField", err)
	}
	if !strings.HasSuffix(err.Error(), "APP_ADRR") {
		t.Errorf("error should name the variable: %v", err)
	}
}

func TestStrictEnvNeedsPrefix(t *testing.T) {
	_, err := cnfg.Parse(Strictly{}, cnfg.EnvFrom[Strictly]("", []string{"ADDR=:1"}, cnfg.Strict))
	if !errors.Is(err, cnfg.ErrNoPrefix) {
		t.Errorf("got %v, want ErrNoPrefix", err)
	}
}

func TestStrictDir(t *testing.T) {
	dir := writeDir(t, map[string]string{"10-base.conf": `{"addr": ":1"}`, "20-typo.conf": `{"adrr": ":2"}`})

	_, err := cnfg.Parse(Strictly{}, cnfg.DecodeDir[Strictly](json.Unmarshal, dir, cnfg.Strict))
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

	if _, err := cnfg.Parse(StrictEmbedded{}, cnfg.Decode[StrictEmbedded](json.Unmarshal, file, cnfg.Strict)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	typo := writeFile(t, "typo.json", `{"log-lvl": "warn"}`)
	_, err := cnfg.Parse(StrictEmbedded{}, cnfg.Decode[StrictEmbedded](json.Unmarshal, typo, cnfg.Strict))
	if !errors.Is(err, cnfg.ErrUnknownField) {
		t.Fatalf("got %v, want ErrUnknownField", err)
	}
}

func TestOnUnknownCollects(t *testing.T) {
	file := writeFile(t, "extras.json", `{"addr": ":1", "worker_cont": 8, "server": {"tls-cret": "a.pem"}}`)

	var seen []cnfg.Unknown
	collect := cnfg.OnUnknown(func(u cnfg.Unknown) error {
		seen = append(seen, u)
		return nil
	})

	cfg, err := cnfg.Parse(Strictly{},
		cnfg.Decode[Strictly](json.Unmarshal, file, collect),
		cnfg.EnvFrom[Strictly]("APP", []string{"APP_ADDR=:2", "APP_ADRR=:3"}, collect),
	)
	if err != nil {
		t.Fatalf("a handler that returns nil should not stop the parse: %v", err)
	}
	assertEqual(t, "parsing went on", ":2", cfg.Addr)

	if len(seen) != 3 {
		t.Fatalf("got %d reports, want one per key: %+v", len(seen), seen)
	}

	assertEqual(t, "file source", file, seen[0].Source)
	assertEqual(t, "keys come in order", "server.tls-cret", seen[0].Key)
	assertEqual(t, "nested value", "a.pem", seen[0].Value)
	assertEqual(t, "second key", "worker_cont", seen[1].Key)
	assertEqual(t, "number value", 8.0, seen[1].Value)
	assertEqual(t, "env source", "APP", seen[2].Source)
	assertEqual(t, "env key", "APP_ADRR", seen[2].Key)
	assertEqual(t, "env value", ":3", seen[2].Value)
}

func TestOnUnknownStaysQuietWhenThereIsNothing(t *testing.T) {
	file := writeFile(t, "clean.json", `{"addr": ":1"}`)

	calls := 0
	count := cnfg.OnUnknown(func(cnfg.Unknown) error {
		calls++
		return nil
	})

	if _, err := cnfg.Parse(Strictly{},
		cnfg.Decode[Strictly](json.Unmarshal, file, count),
		cnfg.EnvFrom[Strictly]("APP", []string{"APP_ADDR=:2"}, count),
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "handler calls", 0, calls)
}

func TestOnUnknownErrorStopsTheParse(t *testing.T) {
	file := writeFile(t, "extras.json", `{"worker_cont": 8}`)
	errMine := errors.New("not having it")

	_, err := cnfg.Parse(Strictly{}, cnfg.Decode[Strictly](json.Unmarshal, file, cnfg.OnUnknown(func(cnfg.Unknown) error {
		return errMine
	})))
	if !errors.Is(err, errMine) {
		t.Fatalf("got %v, want the handler error", err)
	}
}

func TestOnUnknownPerFileInDir(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"10-base.conf":  `{"addr": ":1"}`,
		"20-typo.conf":  `{"adrr": ":2"}`,
		"30-other.conf": `{"nope": true}`,
	})

	var sources []string
	_, err := cnfg.Parse(Strictly{}, cnfg.DecodeDir[Strictly](json.Unmarshal, dir, cnfg.OnUnknown(func(u cnfg.Unknown) error {
		sources = append(sources, filepath.Base(u.Source)+":"+u.Key)
		return nil
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "one report per key, per file", "20-typo.conf:adrr, 30-other.conf:nope", strings.Join(sources, ", "))
}

func TestLogUnknown(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{ReplaceAttr: dropTime}))

	old := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(old) })

	file := writeFile(t, "logged.json", `{"addr": ":1", "worker_cont": 8}`)

	cfg, err := cnfg.Parse(Strictly{}, cnfg.Decode[Strictly](json.Unmarshal, file, cnfg.LogUnknown))
	if err != nil {
		t.Fatalf("logging should not stop the parse: %v", err)
	}
	assertEqual(t, "parsing went on", ":1", cfg.Addr)

	want := `level=WARN msg="config key with no field" source=` + file + " key=worker_cont\n"
	assertEqual(t, "log line", want, buf.String())
}

func dropTime(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return a
}
