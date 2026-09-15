package validator_test

import (
	"strings"
	"testing"

	v10 "github.com/go-playground/validator/v10"

	"github.com/go-cnfg/cnfg"
	"github.com/go-cnfg/cnfg/validator"
)

type Config struct {
	Addr    string `validate:"required,hostname_port"`
	Workers int    `validate:"gte=1,lte=100"`
}

func defaults() Config { return Config{Addr: ":8080", Workers: 4} }

func TestValidate(t *testing.T) {
	cfg, err := cnfg.Parse(defaults(),
		cnfg.EnvFrom[Config]("APP", []string{"APP_WORKERS=8"}),
		validator.Validate[Config](),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workers != 8 {
		t.Errorf("workers: got %d", cfg.Workers)
	}
}

func TestValidateRejects(t *testing.T) {
	_, err := cnfg.Parse(defaults(),
		cnfg.EnvFrom[Config]("APP", []string{"APP_WORKERS=0"}),
		validator.Validate[Config](),
	)
	if err == nil {
		t.Fatal("workers below the minimum should not be accepted")
	}
	if !strings.Contains(err.Error(), "Workers") {
		t.Errorf("error should name the field: %v", err)
	}
}

func TestWith(t *testing.T) {
	v := v10.New()
	v.SetTagName("check")

	type tagged struct {
		Addr string `check:"required"`
	}

	if _, err := cnfg.Parse(tagged{Addr: ":8080"}, validator.With[tagged](v)); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if _, err := cnfg.Parse(tagged{}, validator.With[tagged](v)); err == nil {
		t.Error("empty addr should not be accepted")
	}
}
