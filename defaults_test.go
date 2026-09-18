package cnfg_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/go-cnfg/cnfg"
)

type Embedded struct {
	Name string
}

// Shared has the kinds of fields a copy of a struct shares with the original: an embedded
// pointer, pointers to a leaf and to a struct, a map, a slice and an any holding a map.
type Shared struct {
	*Embedded

	Port   *int
	When   *time.Time
	Server *Server
	Tags   map[string]string
	Hosts  []string
	Any    any
}

func shared() Shared {
	port := 80
	when := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	return Shared{
		Embedded: &Embedded{Name: "base"},
		Port:     &port,
		When:     &when,
		Server:   &Server{TLSCert: "cert.pem", MaxConns: 10},
		Tags:     map[string]string{"env": "dev"},
		Hosts:    []string{"localhost"},
		Any:      map[string]string{"k": "v"},
	}
}

func describe(s Shared) string {
	return fmt.Sprintf("name=%s port=%d when=%s server=%+v tags=%v hosts=%v any=%v",
		s.Name, *s.Port, s.When.Format(time.RFC3339), *s.Server, s.Tags, s.Hosts, s.Any)
}

func TestDefaultsAreLeftAlone(t *testing.T) {
	file := writeFile(t, "shared.json", `{
		"name": "other",
		"port": 90,
		"when": "2026-01-01T00:00:00Z",
		"server": {"tls-cert": "other.pem"},
		"tags": {"team": "a"},
		"hosts": ["a"]
	}`)

	tests := []struct {
		name   string
		source cnfg.Source
	}{
		{name: "file", source: cnfg.File(json.Unmarshal, file)},
		{name: "env", source: env(
			"NAME=other", "PORT=90", "WHEN=2026-01-01T00:00:00Z", "SERVER_TLS_CERT=other.pem", "HOSTS=a",
		)},
		{name: "flags", source: flags(
			"-name", "other", "-port", "90", "-when", "2026-01-01T00:00:00Z", "-server-tls-cert", "other.pem", "-hosts", "a",
		)},
		{name: "own source", source: cnfg.Func(func(cfg Shared) (Shared, error) {
			labels, ok := cfg.Any.(map[string]string)
			if !ok {
				return cfg, errors.New("any should hold the map from the defaults")
			}
			cfg.Name = "other"
			*cfg.Port = 90
			*cfg.When = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			cfg.Server.TLSCert = "other.pem"
			cfg.Tags["team"] = "a"
			cfg.Hosts[0] = "a"
			labels["k"] = "changed"
			return cfg, nil
		})},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defaults := shared()
			cfg, err := cnfg.Parse(defaults, tc.source)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			assertEqual(t, "name", "other", cfg.Name)
			assertEqual(t, "port", 90, *cfg.Port)
			assertEqual(t, "when", 2026, cfg.When.Year())
			assertEqual(t, "server", "other.pem", cfg.Server.TLSCert)
			if !reflect.DeepEqual(defaults, shared()) {
				t.Errorf("defaults were written to:\n got %s\nwant %s", describe(defaults), describe(shared()))
			}
		})
	}
}

func TestSameDefaultsParsedTwice(t *testing.T) {
	first := writeFile(t, "first.json", `{"port": 90, "server": {"tls-cert": "other.pem"}, "tags": {"team": "a"}}`)
	second := writeFile(t, "second.json", `{"tags": {"team": "b"}}`)
	defaults := shared()

	one, err := cnfg.Parse(defaults, cnfg.File(json.Unmarshal, first))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	two, err := cnfg.Parse(defaults, cnfg.File(json.Unmarshal, second))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "port is back to the default", 80, *two.Port)
	assertEqual(t, "server is back to the default", "cert.pem", two.Server.TLSCert)
	if want := map[string]string{"env": "dev", "team": "b"}; !reflect.DeepEqual(want, two.Tags) {
		t.Errorf("tags: got %v, want %v", two.Tags, want)
	}
	assertEqual(t, "first result keeps its team", "a", one.Tags["team"])
	assertEqual(t, "first result keeps its port", 90, *one.Port)
}
