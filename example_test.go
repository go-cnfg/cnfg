package cnfg_test

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-cnfg/cnfg"
)

type AppConfig struct {
	Addr    string        `usage:"address to listen on"`
	Timeout time.Duration `usage:"request timeout"`
	Debug   bool          `usage:"enable debug logging"`
}

func ExampleParse() {
	defaults := AppConfig{
		Addr:    ":8080",
		Timeout: 5 * time.Second,
	}

	cfg, err := cnfg.Parse(defaults,
		cnfg.File(json.Unmarshal, "app.json"),
		cnfg.EnvFrom("APP", []string{"APP_TIMEOUT=1m"}),
		flags("-debug"),
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("%+v\n", cfg)
	// Output: {Addr::8080 Timeout:1m0s Debug:true}
}

func ExampleFunc() {
	withDefaultPort := func(cfg AppConfig) (AppConfig, error) {
		if cfg.Addr == "" {
			return cfg, fmt.Errorf("addr is required")
		}
		if cfg.Addr[0] == ':' {
			cfg.Addr = "localhost" + cfg.Addr
		}
		return cfg, nil
	}

	cfg, err := cnfg.Parse(AppConfig{Addr: ":8080"}, cnfg.Func(withDefaultPort))
	fmt.Println(cfg.Addr, err)
	// Output: localhost:8080 <nil>
}

func ExampleFlagSet() {
	set := flag.NewFlagSet("app", flag.ContinueOnError)
	set.SetOutput(os.Stdout)
	args := []string{"-h"}

	_, _ = cnfg.Parse(AppConfig{Addr: ":8080", Timeout: 5 * time.Second},
		cnfg.FileFromFlag(json.Unmarshal, set, "config", args),
		cnfg.FlagSet(set, args),
	)

	// Output:
	// Usage of app:
	//   -config string
	//     	path to config file
	//   -addr string
	//     	address to listen on (default :8080)
	//   -timeout duration
	//     	request timeout (default 5s)
	//   -debug
	//     	enable debug logging
}

type ServerConfig struct {
	Addr    string `usage:"address to listen on"`
	Workers int    `usage:"number of workers"`
}

func ExampleFunc_validation() {
	validate := func(cfg ServerConfig) (ServerConfig, error) {
		if cfg.Workers < 1 {
			return cfg, fmt.Errorf("workers must be at least 1, got %d", cfg.Workers)
		}
		return cfg, nil
	}
	defaults := ServerConfig{Addr: ":8080", Workers: 4}

	_, err := cnfg.Parse(defaults,
		cnfg.EnvFrom("APP", []string{"APP_WORKERS=0"}),
		cnfg.Func(validate),
	)
	fmt.Println(err)

	cfg, err := cnfg.Parse(defaults,
		cnfg.EnvFrom("APP", []string{"APP_WORKERS=8"}),
		cnfg.Func(validate),
	)
	fmt.Println(cfg.Workers, err)

	// Output:
	// workers must be at least 1, got 0
	// 8 <nil>
}

func ExampleGlob() {
	dir, _ := os.MkdirTemp("", "conf.d")
	defer func() { _ = os.RemoveAll(dir) }()

	for name, content := range map[string]string{
		"10-base.conf":  `{"addr": ":1111", "timeout": "1m"}`,
		"99-local.conf": `{"addr": ":9999"}`,
	} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
	}

	cfg, err := cnfg.Parse(AppConfig{Addr: ":8080"},
		cnfg.Glob(json.Unmarshal, filepath.Join(dir, "*.conf")),
	)
	fmt.Printf("%+v %v\n", cfg, err)
	// Output: {Addr::9999 Timeout:1m0s Debug:false} <nil>
}

func ExampleFileStrict() {
	type Config struct {
		Addr string
	}

	file := filepath.Join(os.TempDir(), "cnfg-strict.json")
	_ = os.WriteFile(file, []byte(`{"addr": ":8080", "adrr": ":9090"}`), 0o600)
	defer func() { _ = os.Remove(file) }()

	_, err := cnfg.Parse(Config{}, cnfg.FileStrict(json.Unmarshal, file))
	fmt.Println(strings.TrimPrefix(err.Error(), file+": "))

	cfg, err := cnfg.Parse(Config{}, cnfg.File(json.Unmarshal, file))
	fmt.Println(cfg.Addr, err)

	// Output:
	// no field for adrr
	// :8080 <nil>
}

func ExampleRequire() {
	type Config struct {
		Addr    string `cnfg:",require"`
		Workers int
	}

	_, err := cnfg.Parse(Config{Workers: 4},
		cnfg.EnvFrom("APP", []string{"APP_WORKERS=8"}),
		cnfg.Require(),
	)
	fmt.Println(err)

	cfg, err := cnfg.Parse(Config{Workers: 4},
		cnfg.EnvFrom("APP", []string{"APP_ADDR=:8080"}),
		cnfg.Require(),
	)
	fmt.Println(cfg.Addr, err)

	// Output:
	// required field not set: addr
	// :8080 <nil>
}

// tempFile writes content to a file in the temp dir and returns its path and the cleanup.
func tempFile(name, content string) (string, func()) {
	path := filepath.Join(os.TempDir(), name)
	_ = os.WriteFile(path, []byte(content), 0o600)
	return path, func() { _ = os.Remove(path) }
}

func ExampleFile() {
	file, cleanup := tempFile("cnfg-file.json", `{"addr": ":9090", "timeout": "30s"}`)
	defer cleanup()

	cfg, err := cnfg.Parse(AppConfig{Addr: ":8080"}, cnfg.File(json.Unmarshal, file))
	fmt.Printf("%+v %v\n", cfg, err)

	cfg, err = cnfg.Parse(AppConfig{Addr: ":8080"}, cnfg.File(json.Unmarshal, "not-there.json"))
	fmt.Printf("%+v %v\n", cfg, err)

	// Output:
	// {Addr::9090 Timeout:30s Debug:false} <nil>
	// {Addr::8080 Timeout:0s Debug:false} <nil>
}

func ExampleFileFromFlag() {
	file, cleanup := tempFile("cnfg-flag.json", `{"addr": ":9090"}`)
	defer cleanup()

	set := flag.NewFlagSet("app", flag.ContinueOnError)
	args := []string{"-config", file, "-debug"}

	cfg, err := cnfg.Parse(AppConfig{Addr: ":8080"},
		cnfg.FileFromFlag(json.Unmarshal, set, "config", args),
		cnfg.FlagSet(set, args),
	)
	fmt.Printf("%+v %v\n", cfg, err)
	// Output: {Addr::9090 Timeout:0s Debug:true} <nil>
}

func ExampleFileFromEnv() {
	file, cleanup := tempFile("cnfg-env.json", `{"addr": ":9090"}`)
	defer cleanup()

	_ = os.Setenv("APP_CONFIG", file)
	defer func() { _ = os.Unsetenv("APP_CONFIG") }()

	cfg, err := cnfg.Parse(AppConfig{Addr: ":8080"},
		cnfg.FileFromEnv(json.Unmarshal, "APP_CONFIG"),
		cnfg.EnvFrom("APP", []string{"APP_DEBUG=true"}),
	)
	fmt.Printf("%+v %v\n", cfg, err)
	// Output: {Addr::9090 Timeout:0s Debug:true} <nil>
}

func ExampleEnv() {
	type Config struct {
		Addr   string
		Server struct {
			TLSCert string
		}
	}

	_ = os.Setenv("APP_ADDR", ":9090")
	_ = os.Setenv("APP_SERVER_TLS_CERT", "cert.pem")
	defer func() {
		_ = os.Unsetenv("APP_ADDR")
		_ = os.Unsetenv("APP_SERVER_TLS_CERT")
	}()

	cfg, err := cnfg.Parse(Config{Addr: ":8080"}, cnfg.Env("APP"))
	fmt.Println(cfg.Addr, cfg.Server.TLSCert, err)
	// Output: :9090 cert.pem <nil>
}

func ExampleEnvFrom() {
	type Config struct {
		Hosts   []string
		Timeout time.Duration
	}

	cfg, err := cnfg.Parse(Config{},
		cnfg.EnvFrom("APP", []string{"APP_HOSTS=a,b,c", "APP_TIMEOUT=1m"}),
	)
	fmt.Println(cfg.Hosts, cfg.Timeout, err)
	// Output: [a b c] 1m0s <nil>
}

func ExampleEnvStrict() {
	type Config struct {
		Addr string
	}

	_ = os.Setenv("APP_ADRR", ":9090")
	defer func() { _ = os.Unsetenv("APP_ADRR") }()

	_, err := cnfg.Parse(Config{}, cnfg.EnvStrict("APP"))
	fmt.Println(err)

	_, err = cnfg.Parse(Config{}, cnfg.EnvStrict(""))
	fmt.Println(err)

	// Output:
	// APP: no field for APP_ADRR
	// strict env needs a prefix
}

func ExampleEnvFromStrict() {
	type Config struct {
		Addr string
	}

	environ := []string{"APP_ADDR=:8080", "APP_ADRR=:9090", "PATH=/bin"}

	_, err := cnfg.Parse(Config{}, cnfg.EnvFromStrict("APP", environ))
	fmt.Println(err)

	cfg, err := cnfg.Parse(Config{}, cnfg.EnvFrom("APP", environ))
	fmt.Println(cfg.Addr, err)

	// Output:
	// APP: no field for APP_ADRR
	// :8080 <nil>
}

func ExampleFlags() {
	args := os.Args
	os.Args = []string{"app", "-addr", ":9090", "-timeout", "1m"}
	defer func() { os.Args = args }()

	cfg, err := cnfg.Parse(AppConfig{Addr: ":8080"}, cnfg.Flags())
	fmt.Printf("%+v %v\n", cfg, err)
	// Output: {Addr::9090 Timeout:1m0s Debug:false} <nil>
}

func ExampleGlobStrict() {
	dir, _ := os.MkdirTemp("", "conf.d")
	defer func() { _ = os.RemoveAll(dir) }()

	for name, content := range map[string]string{
		"10-base.conf": `{"addr": ":1111"}`,
		"20-typo.conf": `{"adrr": ":2222"}`,
	} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
	}

	_, err := cnfg.Parse(AppConfig{},
		cnfg.GlobStrict(json.Unmarshal, filepath.Join(dir, "*.conf")),
	)
	fmt.Println(strings.TrimPrefix(err.Error(), dir+string(filepath.Separator)))
	// Output: 20-typo.conf: no field for adrr
}

func ExampleFileFromFlagStrict() {
	file, cleanup := tempFile("cnfg-flag-strict.json", `{"addr": ":9090", "adrr": ":9091"}`)
	defer cleanup()

	set := flag.NewFlagSet("app", flag.ContinueOnError)
	args := []string{"-config", file}

	_, err := cnfg.Parse(AppConfig{},
		cnfg.FileFromFlagStrict(json.Unmarshal, set, "config", args),
		cnfg.FlagSet(set, args),
	)
	fmt.Println(strings.TrimPrefix(err.Error(), file+": "))
	// Output: no field for adrr
}

func ExampleFileFromEnvStrict() {
	file, cleanup := tempFile("cnfg-env-strict.json", `{"addr": ":9090", "adrr": ":9091"}`)
	defer cleanup()

	_ = os.Setenv("APP_CONFIG", file)
	defer func() { _ = os.Unsetenv("APP_CONFIG") }()

	_, err := cnfg.Parse(AppConfig{}, cnfg.FileFromEnvStrict(json.Unmarshal, "APP_CONFIG"))
	fmt.Println(strings.TrimPrefix(err.Error(), file+": "))
	// Output: no field for adrr
}

func ExampleDecoder() {
	// keyValue reads one key=value pair per line, which is all a Decoder has to do.
	keyValue := func(data []byte, v any) error {
		tree, _ := v.(*map[string]any)
		for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
			if key, value, ok := strings.Cut(line, "="); ok {
				(*tree)[key] = value
			}
		}
		return nil
	}

	file, cleanup := tempFile("cnfg.conf", "addr=:9090\ntimeout=45s\n")
	defer cleanup()

	cfg, err := cnfg.Parse(AppConfig{}, cnfg.File(keyValue, file))
	fmt.Printf("%+v %v\n", cfg, err)
	// Output: {Addr::9090 Timeout:45s Debug:false} <nil>
}

func Example_names() {
	type Config struct {
		ListenAddr string
		Server     struct {
			TLSCert string
		}
		LogLevel string `cnfg:"level"`
		Secret   string `cnfg:"-"`
	}

	cfg, err := cnfg.Parse(Config{Secret: "kept"},
		cnfg.EnvFrom("APP", []string{"APP_LISTEN_ADDR=:9090", "APP_SERVER_TLS_CERT=cert.pem", "APP_SECRET=leaked"}),
		flags("-level", "debug"),
	)
	fmt.Println(cfg.ListenAddr, cfg.Server.TLSCert, cfg.LogLevel, cfg.Secret, err)
	// Output: :9090 cert.pem debug kept <nil>
}

func Example_invalidValue() {
	type Config struct {
		Workers int
		Timeout time.Duration
	}

	_, err := cnfg.Parse(Config{}, cnfg.EnvFrom("APP", []string{"APP_WORKERS=many"}))
	fmt.Println(err)
	fmt.Println(errors.Is(err, cnfg.ErrInvalidValue))

	_, err = cnfg.Parse(Config{}, flags("-timeout", "soon"))
	fmt.Println(err)
	fmt.Println(errors.Is(err, cnfg.ErrParseFlags))

	// Output:
	// invalid value for APP_WORKERS: strconv.ParseInt: parsing "many": invalid syntax
	// true
	// failed to parse flags: invalid value "soon" for flag -timeout: time: invalid duration "soon"
	// true
}
