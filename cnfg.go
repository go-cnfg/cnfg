// Package cnfg is a dead simple zero dependency config parser.
//
// Config is a plain struct that you fill with your defaults and hand to Parse
// together with the sources you want to read. Sources are applied in the order
// they are given, so the last one wins:
//
//	cfg, err := cnfg.Parse(Config{Addr: ":8080"},
//		cnfg.File(json.Unmarshal, "app.json"),
//		cnfg.Env("APP"),
//		cnfg.Flags(),
//	)
//
// A config file format is the decoder you pass in, so the format library stays your own
// dependency and cnfg itself needs nothing outside the standard library.
package cnfg

import (
	"fmt"
	"reflect"

	"github.com/go-cnfg/cnfg/strerr"
)

const (
	// NameTag overrides the name that is generated from the field name,
	// for example `cnfg:"addr"`. Value "-" leaves the field out of every source,
	// and the require option after the name, `cnfg:"addr,require"` or
	// `cnfg:",require"`, has Require check that the field was set.
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
	ErrRequired        = strerr.Error("required field not set")
	ErrWrongType       = strerr.Error("source was made for another config type")
)

// Decoder decodes config file contents into a *map[string]any, which is then applied
// on top of the config struct. encoding/json, go.yaml.in/yaml/v3 and github.com/BurntSushi/toml
// all satisfy it.
type Decoder func(data []byte, v any) error

// Source reads one config source on top of the config being parsed. File, Env, Flags and
// friends return one, and Func turns anything that fills or validates a config into one.
// A Source carries no config type of its own, so the sources you hand to Parse need no
// type argument and the config type is inferred from the defaults.
type Source interface {
	apply(v reflect.Value) error
}

// sourceFunc is a Source written as a function over the config being parsed.
type sourceFunc func(v reflect.Value) error

func (f sourceFunc) apply(v reflect.Value) error { return f(v) }

// Parse applies the sources in the given order on top of a deep copy of defaults and returns
// the result. Fields that no source touched keep the value they had in defaults.
// The zero value of T is returned together with the error when a source fails.
func Parse[T any](defaults T, sources ...Source) (T, error) {
	cfg := clone(defaults)

	v := reflect.ValueOf(&cfg).Elem()
	if v.Kind() != reflect.Struct {
		var zero T
		return zero, fmt.Errorf("%w, got %T", ErrNotStruct, cfg)
	}

	for _, s := range sources {
		if err := s.apply(v); err != nil {
			var zero T
			return zero, err
		}
	}
	return cfg, nil
}

// Func turns a function that fills or validates a config into a Source, so that your own
// step can sit in the same list as the ones cnfg provides:
//
//	cnfg.Parse(Config{}, cnfg.Env("APP"), cnfg.Func(validate))
//
// The config type is inferred from fn. Handing the result to a Parse of another type
// fails with ErrWrongType.
func Func[T any](fn func(T) (T, error)) Source {
	return sourceFunc(func(v reflect.Value) error {
		cfg, ok := v.Interface().(T)
		if !ok {
			return fmt.Errorf("%w: %s, not %s", ErrWrongType, reflect.TypeFor[T](), v.Type())
		}

		out, err := fn(cfg)
		if err != nil {
			return err
		}
		v.Set(reflect.ValueOf(out))
		return nil
	})
}
