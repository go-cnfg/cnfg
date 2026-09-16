package cnfg

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// unknownField wraps the first key a strict source could not place, and is nil when
// there is none. Keys are sorted so the same file gives the same error every time.
func unknownField(source string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	slices.Sort(keys)
	return fmt.Errorf("%s: %w %s", source, ErrUnknownField, keys[0])
}

// unknownKeys lists the keys of tree that no field of the struct behind v reads,
// with the names the file itself used, nested keys joined with a dot.
func unknownKeys(v reflect.Value, tree map[string]any, prefix string) []string {
	fields := treeFields(v)

	var unknown []string
	for key, x := range tree {
		fv, ok := fields[normalize(key)]
		if !ok {
			unknown = append(unknown, prefix+key)
			continue
		}
		unknown = append(unknown, unknownIn(fv, x, prefix+key+".")...)
	}
	return unknown
}

// treeFields maps every name a struct answers to, its own and the Go one, to its value.
// Embedded structs are flattened the same way the file source reads them.
func treeFields(v reflect.Value) map[string]reflect.Value {
	fields := map[string]reflect.Value{}

	t := v.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		tag := parseTag(sf)
		if !sf.IsExported() || tag.skip {
			continue
		}

		fv := v.Field(i)
		if sf.Anonymous && !tag.named && !isLeaf(fv.Type()) {
			if sv, ok := structValue(fv); ok {
				for name, ev := range treeFields(sv) {
					fields[name] = ev
				}
				continue
			}
		}

		fields[normalize(tag.name)] = fv
		fields[normalize(sf.Name)] = fv
	}
	return fields
}

// unknownIn walks into the value a key was matched to, so nested keys are checked too.
func unknownIn(v reflect.Value, x any, prefix string) []string {
	if v.Kind() == reflect.Pointer && !v.IsNil() {
		v = v.Elem()
	}

	switch {
	case v.Kind() == reflect.Struct && !isLeaf(v.Type()):
		if tree, ok := x.(map[string]any); ok {
			return unknownKeys(v, tree, prefix)
		}
	case v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Struct:
		list, _ := x.([]any)

		var unknown []string
		for i, item := range list {
			elem := reflect.New(v.Type().Elem()).Elem()
			unknown = append(unknown, unknownIn(elem, item, fmt.Sprintf("%s[%d].", strings.TrimSuffix(prefix, "."), i))...)
		}
		return unknown
	}
	return nil
}
