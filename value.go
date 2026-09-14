package cnfg

import (
	"encoding"
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var (
	textUnmarshaler = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	flagValuer      = reflect.TypeOf((*flag.Value)(nil)).Elem()
	durationType    = reflect.TypeOf(time.Duration(0))
	byteSliceType   = reflect.TypeOf([]byte(nil))
)

// typeNames are the value types shown in the flag usage output.
var typeNames = map[reflect.Kind]string{
	reflect.Bool:    "",
	reflect.String:  "string",
	reflect.Int:     "int",
	reflect.Int8:    "int",
	reflect.Int16:   "int",
	reflect.Int32:   "int",
	reflect.Int64:   "int",
	reflect.Uint:    "uint",
	reflect.Uint8:   "uint",
	reflect.Uint16:  "uint",
	reflect.Uint32:  "uint",
	reflect.Uint64:  "uint",
	reflect.Float32: "float",
	reflect.Float64: "float",
}

// isLeaf reports whether values of type t can be set from a single string.
func isLeaf(t reflect.Type) bool {
	if isSetter(t) {
		return true
	}

	switch t.Kind() {
	case reflect.Pointer, reflect.Slice:
		return isLeaf(t.Elem())
	default:
		_, ok := typeNames[t.Kind()]
		return ok
	}
}

func isSetter(t reflect.Type) bool {
	pt := reflect.PointerTo(t)
	return t.Implements(textUnmarshaler) || t.Implements(flagValuer) ||
		pt.Implements(textUnmarshaler) || pt.Implements(flagValuer)
}

func elemType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func isBool(t reflect.Type) bool { return elemType(t).Kind() == reflect.Bool }

// typeName is the value type shown in the flag usage output.
func typeName(t reflect.Type) string {
	t = elemType(t)

	switch {
	case t == durationType:
		return "duration"
	case t == byteSliceType:
		return "string"
	case t.Kind() == reflect.Slice:
		return strings.TrimSpace(typeName(t.Elem()) + " list")
	}

	if name, ok := typeNames[t.Kind()]; ok {
		return name
	}
	return "value"
}

func setValue(v reflect.Value, s string) error {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		if u, ok := v.Interface().(encoding.TextUnmarshaler); ok {
			return u.UnmarshalText([]byte(s))
		}
		return setValue(v.Elem(), s)
	}

	if v.CanAddr() {
		switch u := v.Addr().Interface().(type) {
		case flag.Value:
			return u.Set(s)
		case encoding.TextUnmarshaler:
			return u.UnmarshalText([]byte(s))
		}
	}

	return setBasic(v, s)
}

func setBasic(v reflect.Value, s string) error {
	if v.Type() == durationType {
		d, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		v.SetInt(int64(d))
		return nil
	}

	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
		return nil
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		v.SetBool(b)
		return err
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(s, 0, v.Type().Bits())
		v.SetInt(i)
		return err
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(s, 0, v.Type().Bits())
		v.SetUint(u)
		return err
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(s, v.Type().Bits())
		v.SetFloat(f)
		return err
	case reflect.Slice:
		return setSlice(v, s)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedType, v.Type())
	}
}

func setSlice(v reflect.Value, s string) error {
	if v.Type() == byteSliceType {
		v.SetBytes([]byte(s))
		return nil
	}

	parts := []string{}
	if s != "" {
		parts = strings.Split(s, ",")
	}

	slice := reflect.MakeSlice(v.Type(), len(parts), len(parts))
	for i, part := range parts {
		if err := setValue(slice.Index(i), strings.TrimSpace(part)); err != nil {
			return err
		}
	}
	v.Set(slice)
	return nil
}

func stringOf(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}

	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return ""
		}
		return stringOf(v.Elem())
	}

	if s, ok := marshalString(v); ok {
		return s
	}

	switch {
	case v.Type() == durationType:
		return time.Duration(v.Int()).String()
	case v.Type() == byteSliceType:
		return string(v.Bytes())
	case v.Kind() == reflect.Slice:
		return joinSlice(v)
	default:
		return fmt.Sprint(v.Interface())
	}
}

func marshalString(v reflect.Value) (string, bool) {
	if !v.CanAddr() {
		return "", false
	}

	switch m := v.Addr().Interface().(type) {
	case flag.Value:
		return m.String(), true
	case encoding.TextMarshaler:
		if b, err := m.MarshalText(); err == nil {
			return string(b), true
		}
	}
	return "", false
}

func joinSlice(v reflect.Value) string {
	parts := make([]string, v.Len())
	for i := range parts {
		parts[i] = stringOf(v.Index(i))
	}
	return strings.Join(parts, ",")
}

// flagValue binds a config field to the flag package.
type flagValue struct {
	value reflect.Value
}

func (f *flagValue) String() string { return stringOf(f.value) }

func (f *flagValue) Set(s string) error { return setValue(f.value, s) }

func (f *flagValue) IsBoolFlag() bool { return isBool(f.value.Type()) }

// discardValue accepts and throws away a flag value. It is used while looking up
// the config file path, before any of the real values are set.
type discardValue bool

func (d discardValue) String() string   { return "" }
func (d discardValue) Set(string) error { return nil }
func (d discardValue) IsBoolFlag() bool { return bool(d) }
