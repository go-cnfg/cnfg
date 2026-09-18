package cnfg_test

import (
	"errors"
	"flag"
	"io"
	"testing"

	"github.com/go-cnfg/cnfg"
)

type envTagged struct {
	ListenAddr string `env:"LISTENADDR"`
	Renamed    string `cnfg:"other"     env:"THIRD"`
	Plain      string
	Server     struct {
		TLSCert string `env:"CERT"`
		Port    int
	} `env:"SRV"`
}

func TestEnvTag(t *testing.T) {
	cfg, err := cnfg.Parse(envTagged{}, cnfg.EnvFrom("APP", []string{
		"APP_LISTENADDR=:9090",
		"APP_THIRD=third",
		"APP_PLAIN=plain",
		"APP_SRV_CERT=cert.pem",
		"APP_SRV_PORT=443",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "leaf alias", ":9090", cfg.ListenAddr)
	assertEqual(t, "alias beats the cnfg name", "third", cfg.Renamed)
	assertEqual(t, "untagged field", "plain", cfg.Plain)
	assertEqual(t, "alias on a nested struct", "cert.pem", cfg.Server.TLSCert)
	assertEqual(t, "untagged field under an alias", 443, cfg.Server.Port)
}

func TestEnvTagLeavesTheDerivedNameUnread(t *testing.T) {
	cfg, err := cnfg.Parse(envTagged{}, cnfg.EnvFrom("APP", []string{
		"APP_LISTEN_ADDR=:9090",
		"APP_SERVER_TLS_CERT=cert.pem",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "derived leaf name", "", cfg.ListenAddr)
	assertEqual(t, "derived nested name", "", cfg.Server.TLSCert)
}

func TestEnvTagLowerCase(t *testing.T) {
	var cfg struct {
		ListenAddr string `env:"listen-addr"`
	}

	out, err := cnfg.Parse(cfg, cnfg.EnvFrom("APP", []string{"APP_LISTEN_ADDR=:9090"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "normalised like any other name", ":9090", out.ListenAddr)
}

func TestEnvTagStrict(t *testing.T) {
	_, err := cnfg.Parse(envTagged{}, cnfg.EnvFromStrict("APP", []string{"APP_LISTEN_ADDR=:9090"}))
	if !errors.Is(err, cnfg.ErrUnknownField) {
		t.Fatalf("got %v, want ErrUnknownField", err)
	}

	_, err = cnfg.Parse(envTagged{}, cnfg.EnvFromStrict("APP", []string{"APP_LISTENADDR=:9090"}))
	if err != nil {
		t.Errorf("the alias is a known name: %v", err)
	}
}

func TestEnvTagDuplicate(t *testing.T) {
	var cfg struct {
		Addr  string `env:"ADDR"`
		Other string `env:"addr"`
	}

	_, err := cnfg.Parse(cfg, cnfg.EnvFrom("APP", nil))
	if !errors.Is(err, cnfg.ErrDuplicateName) {
		t.Fatalf("got %v, want ErrDuplicateName", err)
	}
}

func TestEnvTagIgnoredByFlagsAndFile(t *testing.T) {
	set := flag.NewFlagSet("app", flag.ContinueOnError)
	set.SetOutput(io.Discard)

	cfg, err := cnfg.Parse(envTagged{}, cnfg.FlagSet(set, []string{"-listen-addr", ":9090", "-server-tls-cert", "cert.pem"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "flags keep the derived name", ":9090", cfg.ListenAddr)
	assertEqual(t, "flags keep the nested derived name", "cert.pem", cfg.Server.TLSCert)
}
