package cnfg

import (
	"fmt"
	"reflect"
	"strings"
)

// assign sets v from a value decoded from a config file.
// Strings go through the same parsing as env vars and flags, so durations,
// net.IP and anything else with UnmarshalText work the same in every source.
func assign(v reflect.Value, x any) error {
	if x == nil {
		return nil
	}

	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		if s, ok := leafString(v, x); ok {
			return setValue(v, s)
		}
		return assign(v.Elem(), x)
	}

	if s, ok := leafString(v, x); ok {
		return setValue(v, s)
	}

	return assignValue(v, x)
}

func assignValue(v reflect.Value, x any) error {
	switch v.Kind() {
	case reflect.Struct:
		return withTree(v, x, assignStruct)
	case reflect.Map:
		return withTree(v, x, assignMap)
	case reflect.Slice:
		list, ok := x.([]any)
		if !ok {
			return typeError(v, x)
		}
		return assignSlice(v, list)
	case reflect.Interface:
		v.Set(reflect.ValueOf(x))
		return nil
	default:
		return assignScalar(v, x)
	}
}

func leafString(v reflect.Value, x any) (string, bool) {
	s, ok := x.(string)
	return s, ok && isLeaf(v.Type())
}

func withTree(v reflect.Value, x any, fn func(reflect.Value, map[string]any) error) error {
	tree, ok := x.(map[string]any)
	if !ok {
		return typeError(v, x)
	}
	return fn(v, tree)
}

func assignScalar(v reflect.Value, x any) error {
	xv := reflect.ValueOf(x)
	switch {
	case xv.Type() == v.Type():
		v.Set(xv)
		return nil
	case isNumber(xv.Kind()) && isNumber(v.Kind()):
		v.Set(xv.Convert(v.Type()))
		return nil
	default:
		return setValue(v, fmt.Sprint(x))
	}
}

func assignStruct(v reflect.Value, tree map[string]any) error {
	norm := make(map[string]any, len(tree))
	for k, x := range tree {
		norm[normalize(k)] = x
	}

	t := v.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		tag := parseTag(sf)
		if !sf.IsExported() || tag.skip {
			continue
		}
		if err := assignField(v.Field(i), sf, tag, tree, norm); err != nil {
			return err
		}
	}
	return nil
}

func assignField(fv reflect.Value, sf reflect.StructField, tag tag, tree, norm map[string]any) error {
	name := tag.name

	if sf.Anonymous && !tag.named && !isLeaf(fv.Type()) {
		if sv, ok := structValue(fv); ok {
			return assignStruct(sv, tree)
		}
	}

	x, ok := norm[normalize(name)]
	if !ok {
		if x, ok = norm[normalize(sf.Name)]; !ok {
			return nil
		}
	}

	if err := assign(fv, x); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func assignMap(v reflect.Value, tree map[string]any) error {
	if v.IsNil() {
		v.Set(reflect.MakeMapWithSize(v.Type(), len(tree)))
	}

	for k, x := range tree {
		key := reflect.New(v.Type().Key()).Elem()
		if err := setValue(key, k); err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
		val := reflect.New(v.Type().Elem()).Elem()
		if err := assign(val, x); err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
		v.SetMapIndex(key, val)
	}
	return nil
}

func assignSlice(v reflect.Value, list []any) error {
	slice := reflect.MakeSlice(v.Type(), len(list), len(list))
	for i, x := range list {
		if err := assign(slice.Index(i), x); err != nil {
			return fmt.Errorf("[%d]: %w", i, err)
		}
	}
	v.Set(slice)
	return nil
}

func typeError(v reflect.Value, x any) error {
	return fmt.Errorf("%w: can't use %T as %s", ErrInvalidValue, x, v.Type())
}

func isNumber(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func normalize(s string) string {
	return strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(s))
}
