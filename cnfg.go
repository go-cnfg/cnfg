// Package cnfg is a dead simple zero dependency config parser.
//
// A config is a plain struct. Fill it with your defaults, hand it to [Parse] along
// with the sources to read, and take the result. Sources are applied in the order
// they are given, so the last one wins:
//
//	cfg, err := cnfg.Parse(Config{Addr: ":8080"},
//		cnfg.File(json.Unmarshal, "app.json"),
//		cnfg.Env("APP"),
//		cnfg.Flags(),
//	)
//
// File formats come from the [Decoder] you pass in, so cnfg itself needs nothing
// outside the standard library.
package cnfg

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"

	"github.com/go-cnfg/cnfg/strerr"
)

const (
	// NameTag names a field in every source, `cnfg:"addr"`, in place of the name
	// derived from the field name. The name "-" leaves the field out, and the
	// option require after it, `cnfg:"addr,require"` or `cnfg:",require"`, is what
	// Require checks.
	NameTag = "cnfg"
	// UsageTag is what a field is documented with in the flag usage output,
	// `usage:"address to listen on"`.
	UsageTag = "usage"
)

// Errors returned by [Parse] and the sources. They come wrapped in the detail of
// what went wrong, so test for them with errors.Is.
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

// Decoder decodes the contents of a config file into a *map[string]any, which is
// then applied on top of the config. The Unmarshal of encoding/json,
// go.yaml.in/yaml/v3 and github.com/BurntSushi/toml all fit, and so does anything
// with the same signature.
type Decoder func(data []byte, v any) error

// Source is one place a config is read from. [File], [Env], [Flags] and the rest
// return one, and [Func] makes one out of a function of your own. A Source carries
// no config type of its own, so [Parse] infers the type from the defaults and no
// call needs a type argument.
type Source interface {
	apply(v reflect.Value) error
}

// sourceFunc is a Source written as a function over the config being parsed.
type sourceFunc func(v reflect.Value) error

func (f sourceFunc) apply(v reflect.Value) error { return f(v) }

// Parse applies the sources to a deep copy of defaults, in the order they are given,
// and returns the result. A field no source touched keeps the value it had in
// defaults, and the defaults themselves are never written to. The first source that
// fails ends the parse, and Parse returns the zero value of T with the error.
// T must be a struct, or Parse fails with [ErrNotStruct].
func Parse[T any](defaults T, sources ...Source) (T, error) {
	cfg := clone(defaults)

	v := reflect.ValueOf(&cfg).Elem()
	if v.Kind() != reflect.Struct {
		var zero T
		return zero, fmt.Errorf("%w, got %T", ErrNotStruct, cfg)
	}

	registerFileFlags(sources)
	for _, s := range sources {
		if err := s.apply(v); err != nil {
			var zero T
			return zero, err
		}
	}
	return cfg, nil
}

// exit is what [MustParse] ends the program with, a variable so the tests can watch it.
var exit = os.Exit

// MustParse is [Parse] for a main that has nowhere to put an error. It gives the config
// when every source read, and otherwise writes the error to stderr and exits with status
// 1:
//
//	func main() {
//		cfg := cnfg.MustParse(Config{Addr: ":8080"}, cnfg.Env("APP"), cnfg.Flags())
//		log.Printf("%+v", cfg)
//	}
//
// The one error that is not a failure is the user asking for the usage output with -h.
// The flag set has written it by then, so MustParse exits with status 0 and says nothing
// of its own. A flag it could not parse is written by the flag set as well, so that one
// exits with status 1 and is likewise left alone. Use [Parse] where the caller has
// somewhere better to put the error.
func MustParse[T any](defaults T, sources ...Source) T {
	cfg, err := Parse(defaults, sources...)
	if err == nil {
		return cfg
	}

	code := 1
	switch {
	case errors.Is(err, flag.ErrHelp):
		code = 0
	case !errors.Is(err, ErrParseFlags):
		fmt.Fprintln(os.Stderr, err)
	}

	exit(code)
	return cfg
}

// Func makes a [Source] out of a function that fills or validates a config, so a
// step of your own can sit in the same list as the rest:
//
//	cnfg.Parse(Config{}, cnfg.Env("APP"), cnfg.Func(validate))
//
// The config type is inferred from fn. A Func given to a [Parse] of another type
// fails with [ErrWrongType].
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
