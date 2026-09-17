package cnfg

import (
	"fmt"
	"reflect"
)

// Require fails when a field tagged `cnfg:",require"` still has its zero value, so put it
// after the sources that are supposed to fill it:
//
//	type Config struct {
//		Addr string `cnfg:",require"`
//	}
//
//	cfg, err := cnfg.Parse(Config{}, cnfg.Env[Config]("APP"), cnfg.Require[Config]())
//
// The error wraps ErrRequired and names the field the way the flag does, server-addr for
// field Addr of struct field Server. The zero value is what counts as not set, so a bool
// or a number that may legitimately be zero is not something to require.
func Require[T any]() Parser[T] {
	return func(cfg T) (T, error) {
		v := reflect.ValueOf(&cfg).Elem()
		if v.Kind() != reflect.Struct {
			return cfg, fmt.Errorf("%w, got %T", ErrNotStruct, cfg)
		}
		if name := missing(v, ""); name != "" {
			return cfg, fmt.Errorf("%w: %s", ErrRequired, name)
		}
		return cfg, nil
	}
}

// missing returns the name of the first required field that is still zero, or "".
func missing(v reflect.Value, prefix string) string {
	t := v.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		tag := parseTag(sf)
		if !sf.IsExported() || tag.skip {
			continue
		}

		fv := v.Field(i)
		name := prefix + tag.name
		if tag.require && fv.IsZero() {
			return name
		}

		sv, ok := nestedStruct(fv)
		if !ok {
			continue
		}
		p := name + "-"
		if sf.Anonymous && !tag.named {
			p = prefix
		}
		if name := missing(sv, p); name != "" {
			return name
		}
	}
	return ""
}

// nestedStruct returns the struct behind v, reading a nil pointer as an empty struct
// rather than allocating one into the config.
func nestedStruct(v reflect.Value) (reflect.Value, bool) {
	if isLeaf(v.Type()) {
		return v, false
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v = reflect.New(v.Type().Elem())
		}
		v = v.Elem()
	}
	return v, v.Kind() == reflect.Struct
}
