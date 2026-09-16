// Package cnfg is a dead simple zero dependency config parser.
//
// Config is a plain struct that you fill with your defaults and hand to Parse
// together with the sources you want to read. Sources are applied in the order
// they are given, so the last one wins:
//
//	cfg, err := cnfg.Parse(Config{Addr: ":8080"},
//		cnfg.Decode(json.Unmarshal, "app.json"),
//		cnfg.Env("APP"),
//		cnfg.Flags(),
//	)
//
// A config file format is the decoder you pass in, so the format library stays your own
// dependency and cnfg itself needs nothing outside the standard library.
package cnfg

import (
	"fmt"

	"github.com/go-cnfg/cnfg/strerr"
)

const (
	// NameTag overrides the name that is generated from the field name,
	// for example `cnfg:"addr"`. Value "-" leaves the field out of every source.
	NameTag = "cnfg"
	// UsageTag documents the field in the flag usage output.
	UsageTag = "usage"
)

const (
	ErrNotStruct       = strerr.Error("config must be a struct")
	ErrDuplicateName   = strerr.Error("duplicate field name")
	ErrReadFile        = strerr.Error("failed to read config file")
	ErrDecodeFile      = strerr.Error("failed to decode config file")
	ErrInvalidValue    = strerr.Error("invalid value")
	ErrParseFlags      = strerr.Error("failed to parse flags")
	ErrUnsupportedType = strerr.Error("unsupported type")
	ErrUnknownField    = strerr.Error("no field for")
	ErrNoPrefix        = strerr.Error("strict env needs a prefix")
	ErrWrongConfig     = strerr.Error("parser was made for another config")
)

// Decoder decodes config file contents into a *map[string]any, which is then applied
// on top of the config struct. encoding/json, go.yaml.in/yaml/v3 and github.com/BurntSushi/toml
// all satisfy it.
type Decoder func(data []byte, v any) error

// Parser reads one config source into the config it is given, which is always a
// pointer to your config struct. Env, Flags and friends return one, and Typed turns
// a function of your own into one.
type Parser func(cfg any) error

// Parse applies the parsers on top of defaults in the given order and returns the result.
// Fields that no parser touched keep the value they had in defaults, and the zero value
// of T is returned together with the error when a parser fails:
//
//	cfg, err := cnfg.Parse(Config{Addr: ":8080"},
//		cnfg.Env("APP"),
//		cnfg.Flags(),
//	)
func Parse[T any](defaults T, parsers ...Parser) (T, error) {
	cfg := defaults
	for _, p := range parsers {
		if err := p(&cfg); err != nil {
			var zero T
			return zero, err
		}
	}
	return cfg, nil
}

// Typed turns a function that takes and returns your config into a Parser, for the
// validation or post processing you write yourself:
//
//	cnfg.Typed(func(cfg Config) (Config, error) { ... })
func Typed[T any](fn func(T) (T, error)) Parser {
	return func(cfg any) error {
		p, ok := cfg.(*T)
		if !ok {
			return fmt.Errorf("%w, got %T", ErrWrongConfig, cfg)
		}

		next, err := fn(*p)
		if err != nil {
			return err
		}

		*p = next
		return nil
	}
}
