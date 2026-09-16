package cnfg_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-cnfg/cnfg"
)

type Config struct {
	Addr     string        `usage:"address to listen on"`
	Workers  int           `json:"worker_count"`
	Debug    bool          `cnfg:"verbose"`
	Timeout  time.Duration `usage:"request timeout"`
	Hosts    []string
	Ratio    float64
	Secret   string `cnfg:"-"`
	IP       net.IP
	Server   Server
	Limits   *Limits
	Metadata map[string]string
}

type Server struct {
	TLSCert  string
	MaxConns uint32
}

type Limits struct {
	RPS int
}

func defaults() Config {
	return Config{
		Addr:    ":8080",
		Workers: 4,
		Timeout: time.Second,
		Hosts:   []string{"localhost"},
		Ratio:   0.5,
		Secret:  "keep",
		Server:  Server{TLSCert: "cert.pem", MaxConns: 10},
		Limits:  &Limits{RPS: 100},
	}
}

// flags parses the given args with a quiet flag set.
func flags[T any](args ...string) cnfg.Parser[T] {
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	return cnfg.FlagSet[T](set, args)
}

func env[T any](environ ...string) cnfg.Parser[T] {
	return cnfg.EnvFrom[T]("", environ)
}

func TestDefaultsAreKept(t *testing.T) {
	cfg, err := cnfg.Parse(defaults(), env[Config](), flags[Config]())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(cfg, defaults()) {
		t.Errorf("got %+v, want %+v", cfg, defaults())
	}
}

func TestNoParsers(t *testing.T) {
	cfg, err := cnfg.Parse(defaults())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(cfg, defaults()) {
		t.Errorf("got %+v, want %+v", cfg, defaults())
	}
}

func TestPrecedence(t *testing.T) {
	file := writeFile(t, "config.json", `{
		"addr": ":1111",
		"worker_count": 1,
		"timeout": "1m",
		"hosts": ["file"],
		"server": {"TLSCert": "file.pem"}
	}`)

	cfg, err := cnfg.Parse(defaults(),
		cnfg.File[Config](json.Unmarshal, file),
		env[Config]("ADDR=:2222", "WORKER_COUNT=2", "TIMEOUT=2m"),
		flags[Config]("-addr", ":3333"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "flag over env", ":3333", cfg.Addr)
	assertEqual(t, "env over file", 2, cfg.Workers)
	assertEqual(t, "env over file", 2*time.Minute, cfg.Timeout)
	assertEqual(t, "file over defaults", "file.pem", cfg.Server.TLSCert)
	assertEqual(t, "untouched default", 0.5, cfg.Ratio)
	assertEqual(t, "untouched default", uint32(10), cfg.Server.MaxConns)
	if !reflect.DeepEqual([]string{"file"}, cfg.Hosts) {
		t.Errorf("hosts: got %v", cfg.Hosts)
	}
}

func TestParserOrder(t *testing.T) {
	cfg, err := cnfg.Parse(defaults(),
		flags[Config]("-addr", ":3333"),
		env[Config]("ADDR=:2222"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "last parser wins", ":2222", cfg.Addr)
}

func TestCustomParser(t *testing.T) {
	errEmptyAddr := errors.New("addr is empty")
	validate := func(cfg Config) (Config, error) {
		if cfg.Addr == "" {
			return cfg, errEmptyAddr
		}
		cfg.Addr = strings.TrimPrefix(cfg.Addr, "http://")
		return cfg, nil
	}

	cfg, err := cnfg.Parse(defaults(), flags[Config]("-addr", "http://:1234"), validate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "parser applied", ":1234", cfg.Addr)

	if _, err := cnfg.Parse(defaults(), flags[Config]("-addr", ""), validate); !errors.Is(err, errEmptyAddr) {
		t.Errorf("got %v, want errEmptyAddr", err)
	}
}

func TestFile(t *testing.T) {
	file := writeFile(t, "config.json", `{
		"addr": ":1111",
		"verbose": true,
		"timeout": "90s",
		"ratio": 2,
		"worker_count": 3,
		"hosts": ["a", "b"],
		"ip": "10.0.0.3",
		"metadata": {"env": "test"},
		"server": {"TLSCert": "file.pem", "max-conns": 33},
		"limits": {"rps": 5}
	}`)

	cfg, err := cnfg.Parse(defaults(), cnfg.File[Config](json.Unmarshal, file))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "string", ":1111", cfg.Addr)
	assertEqual(t, "bool", true, cfg.Debug)
	assertEqual(t, "duration as string", 90*time.Second, cfg.Timeout)
	assertEqual(t, "float", 2.0, cfg.Ratio)
	assertEqual(t, "int", 3, cfg.Workers)
	assertEqual(t, "text unmarshaler", "10.0.0.3", cfg.IP.String())
	assertEqual(t, "map", "test", cfg.Metadata["env"])
	assertEqual(t, "nested by go name", "file.pem", cfg.Server.TLSCert)
	assertEqual(t, "nested by flag name", uint32(33), cfg.Server.MaxConns)
	assertEqual(t, "nested pointer", 5, cfg.Limits.RPS)
	if !reflect.DeepEqual([]string{"a", "b"}, cfg.Hosts) {
		t.Errorf("slice: got %v", cfg.Hosts)
	}
}

func TestFlags(t *testing.T) {
	cfg, err := cnfg.Parse(defaults(), flags[Config](
		"-verbose",
		"-timeout=250ms",
		"-hosts", "a,b, c",
		"-ratio=1.5",
		"-ip", "10.0.0.1",
		"-server-tls-cert", "flag.pem",
		"-server-max-conns", "99",
		"-limits-rps", "7",
		"rest",
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "bool flag without value", true, cfg.Debug)
	assertEqual(t, "duration", 250*time.Millisecond, cfg.Timeout)
	assertEqual(t, "float", 1.5, cfg.Ratio)
	assertEqual(t, "nested", "flag.pem", cfg.Server.TLSCert)
	assertEqual(t, "nested uint", uint32(99), cfg.Server.MaxConns)
	assertEqual(t, "nested pointer", 7, cfg.Limits.RPS)
	assertEqual(t, "text unmarshaler", "10.0.0.1", cfg.IP.String())
	if !reflect.DeepEqual([]string{"a", "b", "c"}, cfg.Hosts) {
		t.Errorf("slice flag: got %v", cfg.Hosts)
	}
}

func TestEnv(t *testing.T) {
	cfg, err := cnfg.Parse(defaults(), env[Config](
		"VERBOSE=true",
		"HOSTS=a,b",
		"IP=10.0.0.2",
		"SERVER_MAX_CONNS=8",
		"LIMITS_RPS=9",
		"SECRET=leaked",
		"METADATA=nope",
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "renamed by tag", true, cfg.Debug)
	assertEqual(t, "nested", uint32(8), cfg.Server.MaxConns)
	assertEqual(t, "nested pointer", 9, cfg.Limits.RPS)
	assertEqual(t, "text unmarshaler", "10.0.0.2", cfg.IP.String())
	assertEqual(t, "field skipped with -", "keep", cfg.Secret)
	if !reflect.DeepEqual([]string{"a", "b"}, cfg.Hosts) {
		t.Errorf("slice env: got %v", cfg.Hosts)
	}
	if cfg.Metadata != nil {
		t.Errorf("map should only be settable from file, got %v", cfg.Metadata)
	}
}

func TestEnvPrefix(t *testing.T) {
	cfg, err := cnfg.Parse(defaults(),
		cnfg.EnvFrom[Config]("app_", []string{"ADDR=:1111", "APP_ADDR=:2222"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "prefixed env", ":2222", cfg.Addr)
}

func TestEnvFromProcess(t *testing.T) {
	t.Setenv("APP_ADDR", ":7777")

	cfg, err := cnfg.Parse(defaults(), cnfg.Env[Config]("APP"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "env from the process", ":7777", cfg.Addr)
}

func TestFileFlag(t *testing.T) {
	file := writeFile(t, "app.json", `{"addr": ":1111", "metadata": {"env": "test"}}`)
	args := []string{"-ratio", "2", "-config", file, "-verbose"}

	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.SetOutput(io.Discard)

	cfg, err := cnfg.Parse(defaults(),
		cnfg.FileFromFlag[Config](json.Unmarshal, set, "config", args),
		cnfg.FlagSet[Config](set, args),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "file from flag", ":1111", cfg.Addr)
	assertEqual(t, "map from file", "test", cfg.Metadata["env"])
	assertEqual(t, "flag before file flag", 2.0, cfg.Ratio)
	assertEqual(t, "flag after file flag", true, cfg.Debug)
}

func TestFileFlagMissing(t *testing.T) {
	args := []string{"-addr", ":1234"}
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.SetOutput(io.Discard)

	cfg, err := cnfg.Parse(defaults(),
		cnfg.FileFromFlag[Config](json.Unmarshal, set, "config", args),
		cnfg.FlagSet[Config](set, args),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "no file given", ":1234", cfg.Addr)
}

func TestMissingFileIsSkipped(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")

	cfg, err := cnfg.Parse(defaults(), cnfg.File[Config](json.Unmarshal, missing))
	if err != nil {
		t.Fatalf("a file that is not there should be a no op: %v", err)
	}
	assertEqual(t, "defaults kept", ":8080", cfg.Addr)
}

func TestUnreadableFileIsAnError(t *testing.T) {
	path := writeFile(t, "locked.json", `{"addr": ":1111"}`)
	if err := os.Chmod(path, 0o000); err != nil {
		t.Skipf("cannot take the read bit away: %v", err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads it anyway")
	}

	_, err := cnfg.Parse(defaults(), cnfg.File[Config](json.Unmarshal, path))
	if !errors.Is(err, cnfg.ErrReadFile) {
		t.Errorf("got %v, want ErrReadFile", err)
	}
}

func TestCustomDecoder(t *testing.T) {
	file := writeFile(t, "config.custom", "addr=:4444")

	decode := func(data []byte, v any) error {
		tree, ok := v.(*map[string]any)
		if !ok {
			return errors.New("wrong type")
		}
		key, val, _ := strings.Cut(string(data), "=")
		(*tree)[key] = val
		return nil
	}
	cfg, err := cnfg.Parse(defaults(), cnfg.File[Config](decode, file))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "custom decoder", ":4444", cfg.Addr)
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name   string
		parser cnfg.Parser[Config]
		want   error
	}{
		{
			name:   "invalid env value",
			parser: env[Config]("WORKER_COUNT=many"),
			want:   cnfg.ErrInvalidValue,
		},
		{
			name:   "invalid flag value",
			parser: flags[Config]("-timeout", "soon"),
			want:   cnfg.ErrParseFlags,
		},
		{
			name:   "unknown flag",
			parser: flags[Config]("-nope"),
			want:   cnfg.ErrParseFlags,
		},
		{
			name:   "help",
			parser: flags[Config]("-h"),
			want:   flag.ErrHelp,
		},
		{
			name:   "broken file",
			parser: cnfg.File[Config](json.Unmarshal, writeFile(t, "config.json", "{")),
			want:   cnfg.ErrDecodeFile,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := cnfg.Parse(defaults(), tc.parser)
			if !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
			if !reflect.DeepEqual(cfg, Config{}) {
				t.Errorf("failed parse should return the zero value, got %+v", cfg)
			}
		})
	}
}

func TestNotStruct(t *testing.T) {
	if _, err := cnfg.Parse(42, flags[int]()); !errors.Is(err, cnfg.ErrNotStruct) {
		t.Errorf("flags: got %v, want ErrNotStruct", err)
	}
	if _, err := cnfg.Parse(42, env[int]()); !errors.Is(err, cnfg.ErrNotStruct) {
		t.Errorf("env: got %v, want ErrNotStruct", err)
	}

	file := writeFile(t, "config.json", "{}")
	if _, err := cnfg.Parse(42, cnfg.File[int](json.Unmarshal, file)); !errors.Is(err, cnfg.ErrNotStruct) {
		t.Errorf("file: got %v, want ErrNotStruct", err)
	}
}

func TestDuplicateName(t *testing.T) {
	type dup struct {
		Addr string
		Host string `cnfg:"addr"`
	}

	if _, err := cnfg.Parse(dup{}, flags[dup]()); !errors.Is(err, cnfg.ErrDuplicateName) {
		t.Errorf("got %v, want ErrDuplicateName", err)
	}
}

func TestEmbedded(t *testing.T) {
	type Common struct {
		LogLevel string
	}
	type Service struct {
		Common

		Name string
	}

	file := writeFile(t, "config.json", `{"log-level": "warn", "name": "from-file"}`)

	cfg, err := cnfg.Parse(Service{},
		cnfg.File[Service](json.Unmarshal, file),
		flags[Service]("-log-level", "debug"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "embedded field", "debug", cfg.LogLevel)
	assertEqual(t, "embedded field from file", "from-file", cfg.Name)
}

func TestFlagSetArgs(t *testing.T) {
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	args := []string{"-addr", ":9999", "one", "two"}

	cfg, err := cnfg.Parse(defaults(), cnfg.FlagSet[Config](set, args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "flag", ":9999", cfg.Addr)
	if want := []string{"one", "two"}; !reflect.DeepEqual(want, set.Args()) {
		t.Errorf("got %v, want %v", set.Args(), want)
	}
}

func TestUsage(t *testing.T) {
	buf := &bytes.Buffer{}
	set := flag.NewFlagSet("app", flag.ContinueOnError)
	set.SetOutput(buf)
	args := []string{"-h"}

	_, _ = cnfg.Parse(defaults(),
		cnfg.FileFromFlag[Config](json.Unmarshal, set, "config", args),
		cnfg.FlagSet[Config](set, args),
	)

	for _, want := range []string{
		"Usage of app:",
		"-config string\n    \tpath to config file",
		"-addr string\n    \taddress to listen on (default :8080)",
		"-server-tls-cert string\n    \t(default cert.pem)",
		"-timeout duration\n    \trequest timeout (default 1s)",
		"-hosts string list\n    \t(default localhost)",
		"-verbose\n",
		"-worker_count int\n    \t(default 4)",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("usage missing %q:\n%s", want, buf.String())
		}
	}
}

func TestUsageWithoutName(t *testing.T) {
	buf := &bytes.Buffer{}
	set := flag.NewFlagSet("", flag.ContinueOnError)
	set.SetOutput(buf)

	_, _ = cnfg.Parse(defaults(), cnfg.FlagSet[Config](set, []string{"-h"}))

	if !strings.HasPrefix(buf.String(), "Usage:\n") {
		t.Errorf("got %q", buf.String())
	}
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertEqual[T comparable](t *testing.T, msg string, want, got T) {
	t.Helper()
	if want != got {
		t.Errorf("%s: got %v, want %v", msg, got, want)
	}
}

func TestEnvIsReadWhenTheParserRuns(t *testing.T) {
	parser := cnfg.Env[Config]("APP")
	t.Setenv("APP_ADDR", ":7778")

	cfg, err := cnfg.Parse(defaults(), parser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "env read at parse time", ":7778", cfg.Addr)
}
