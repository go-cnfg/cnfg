package cnfg_test

import (
	"encoding/json"
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
		cnfg.Optional(cnfg.Decode[AppConfig](json.Unmarshal, "app.json")),
		cnfg.EnvFrom[AppConfig]("APP", []string{"APP_TIMEOUT=1m"}),
		flags[AppConfig]("-debug"),
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("%+v\n", cfg)
	// Output: {Addr::8080 Timeout:1m0s Debug:true}
}

func ExampleParser() {
	withDefaultPort := func(cfg AppConfig) (AppConfig, error) {
		if cfg.Addr == "" {
			return cfg, fmt.Errorf("addr is required")
		}
		if cfg.Addr[0] == ':' {
			cfg.Addr = "localhost" + cfg.Addr
		}
		return cfg, nil
	}

	cfg, err := cnfg.Parse(AppConfig{Addr: ":8080"}, withDefaultPort)
	fmt.Println(cfg.Addr, err)
	// Output: localhost:8080 <nil>
}

func ExampleFlagSet() {
	set := flag.NewFlagSet("app", flag.ContinueOnError)
	set.SetOutput(os.Stdout)
	args := []string{"-h"}

	_, _ = cnfg.Parse(AppConfig{Addr: ":8080", Timeout: 5 * time.Second},
		cnfg.DecodeFlag[AppConfig](json.Unmarshal, set, "config", args),
		cnfg.FlagSet[AppConfig](set, args),
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

func ExampleParser_validation() {
	validate := func(cfg ServerConfig) (ServerConfig, error) {
		if cfg.Workers < 1 {
			return cfg, fmt.Errorf("workers must be at least 1, got %d", cfg.Workers)
		}
		return cfg, nil
	}
	defaults := ServerConfig{Addr: ":8080", Workers: 4}

	_, err := cnfg.Parse(defaults,
		cnfg.EnvFrom[ServerConfig]("APP", []string{"APP_WORKERS=0"}),
		validate,
	)
	fmt.Println(err)

	cfg, err := cnfg.Parse(defaults,
		cnfg.EnvFrom[ServerConfig]("APP", []string{"APP_WORKERS=8"}),
		validate,
	)
	fmt.Println(cfg.Workers, err)

	// Output:
	// workers must be at least 1, got 0
	// 8 <nil>
}

func ExampleDecodeGlob() {
	dir, _ := os.MkdirTemp("", "conf.d")
	defer func() { _ = os.RemoveAll(dir) }()

	for name, content := range map[string]string{
		"10-base.conf":  `{"addr": ":1111", "timeout": "1m"}`,
		"99-local.conf": `{"addr": ":9999"}`,
	} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600)
	}

	cfg, err := cnfg.Parse(AppConfig{Addr: ":8080"},
		cnfg.DecodeGlob[AppConfig](json.Unmarshal, filepath.Join(dir, "*.conf")),
	)
	fmt.Printf("%+v %v\n", cfg, err)
	// Output: {Addr::9999 Timeout:1m0s Debug:false} <nil>
}

func ExampleStrict() {
	type Config struct {
		Addr string
	}

	file := filepath.Join(os.TempDir(), "cnfg-strict.json")
	_ = os.WriteFile(file, []byte(`{"addr": ":8080", "adrr": ":9090"}`), 0o600)
	defer func() { _ = os.Remove(file) }()

	_, err := cnfg.Parse(Config{}, cnfg.Decode[Config](json.Unmarshal, file, cnfg.Strict))
	fmt.Println(strings.TrimPrefix(err.Error(), file+": "))

	cfg, err := cnfg.Parse(Config{}, cnfg.Decode[Config](json.Unmarshal, file))
	fmt.Println(cfg.Addr, err)

	// Output:
	// no field for adrr
	// :8080 <nil>
}

func ExampleOnUnknown() {
	type Config struct {
		Addr string
	}

	file := filepath.Join(os.TempDir(), "cnfg-extras.json")
	_ = os.WriteFile(file, []byte(`{"addr": ":8080", "adrr": ":9090", "workrs": 4}`), 0o600)
	defer func() { _ = os.Remove(file) }()

	cfg, err := cnfg.Parse(Config{}, cnfg.Decode[Config](json.Unmarshal, file, cnfg.OnUnknown(func(u cnfg.Unknown) error {
		fmt.Printf("ignoring %s=%v\n", u.Key, u.Value)
		return nil
	})))
	fmt.Println(cfg.Addr, err)

	// Output:
	// ignoring adrr=:9090
	// ignoring workrs=4
	// :8080 <nil>
}
