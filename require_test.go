package cnfg_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-cnfg/cnfg"
)

type Required struct {
	Base

	Addr   string `cnfg:"addr,require"`
	Port   int
	Server struct {
		TLSCert string `cnfg:",require"`
	}
	Backup *Backup
	Peers  []string          `cnfg:", require"`
	Labels map[string]string `cnfg:",require"`
	Secret string            `cnfg:"-,require"`
	hidden string            `cnfg:",require"`
}

type Base struct {
	Name string `cnfg:",require"`
}

type Backup struct {
	Addr string `cnfg:",require"`
}

func filled() Required {
	cfg := Required{Addr: ":1", Peers: []string{"a"}, Labels: map[string]string{"k": "v"}}
	cfg.Name = "app"
	cfg.Server.TLSCert = "a.pem"
	cfg.Backup = &Backup{Addr: ":2"}
	_ = cfg.hidden
	return cfg
}

func TestRequireAccepts(t *testing.T) {
	if _, err := cnfg.Parse(filled(), cnfg.Require()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireRejects(t *testing.T) {
	tests := []struct {
		name string
		zero func(*Required)
		want string
	}{
		{name: "top level", zero: func(c *Required) { c.Addr = "" }, want: "addr"},
		{name: "embedded", zero: func(c *Required) { c.Name = "" }, want: "name"},
		{name: "nested", zero: func(c *Required) { c.Server.TLSCert = "" }, want: "server-tls-cert"},
		{name: "behind nil pointer", zero: func(c *Required) { c.Backup = nil }, want: "backup-addr"},
		{name: "behind pointer", zero: func(c *Required) { c.Backup.Addr = "" }, want: "backup-addr"},
		{name: "nil slice", zero: func(c *Required) { c.Peers = nil }, want: "peers"},
		{name: "nil map", zero: func(c *Required) { c.Labels = nil }, want: "labels"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := filled()
			tc.zero(&cfg)

			_, err := cnfg.Parse(cfg, cnfg.Require())
			if !errors.Is(err, cnfg.ErrRequired) {
				t.Fatalf("got %v, want ErrRequired", err)
			}
			if !strings.HasSuffix(err.Error(), ": "+tc.want) {
				t.Errorf("error should name %q: %v", tc.want, err)
			}
		})
	}
}

func TestRequireNamesTheFirst(t *testing.T) {
	_, err := cnfg.Parse(Required{}, cnfg.Require())
	if !strings.HasSuffix(err.Error(), ": name") {
		t.Errorf("fields are checked in declaration order, embedded first: %v", err)
	}
}

func TestRequireIsFilledBySources(t *testing.T) {
	cfg := filled()
	cfg.Addr = ""

	got, err := cnfg.Parse(cfg,
		cnfg.EnvFrom("APP", []string{"APP_ADDR=:3"}),
		cnfg.Require(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "addr", ":3", got.Addr)
}

func TestRequireLeavesNilPointersAlone(t *testing.T) {
	cfg := filled()
	cfg.Backup = nil

	got, _ := cnfg.Parse(cfg, cnfg.Require())
	if got.Backup != nil {
		t.Error("checking should not allocate the nested struct")
	}
}

func TestRequireNotStruct(t *testing.T) {
	_, err := cnfg.Parse(42, cnfg.Require())
	if !errors.Is(err, cnfg.ErrNotStruct) {
		t.Errorf("got %v, want ErrNotStruct", err)
	}
}
