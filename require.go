package cnfg

import (
	"fmt"
	"reflect"
)

// Require fails with [ErrRequired] when a field tagged `cnfg:",require"` still holds its
// zero value, so put it last, after the sources meant to fill it:
//
//	type Config struct {
//		Addr string `cnfg:",require"`
//	}
//
//	cfg, err := cnfg.Parse(Config{}, cnfg.Env("APP"), cnfg.Require())
//
// The error names the field the way its flag is named, server-addr for field Addr of
// struct field Server. Zero is what counts as unset, so a bool or a number that may
// legitimately be zero is not something to require.
func Require() Source {
	return sourceFunc(func(v reflect.Value) error {
		if name := missing(v, ""); name != "" {
			return fmt.Errorf("%w: %s", ErrRequired, name)
		}
		return nil
	})
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
