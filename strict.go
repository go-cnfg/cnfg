package cnfg

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// Option changes how a source is read.
type Option func(*options)

type options struct {
	onUnknown func(Unknown) error
}

// Unknown is what a source could not place, with the file it was read from or the
// env prefix it was looked up with as the source.
type Unknown struct {
	Source string
	Keys   []string
}

// OnUnknown hands fn the keys a source could not place, so you can fail on them,
// log them or count them. It is called once per file, and once for the environment,
// only when there is something to report, and the error it returns ends the parse:
//
//	cnfg.Decode[Config](yaml.Unmarshal, path, cnfg.OnUnknown(func(u cnfg.Unknown) error {
//		log.Printf("%s: ignoring %v", u.Source, u.Keys)
//		return nil
//	}))
//
// Keys under a map or an any field are values rather than names, so they are left
// alone. Env needs a prefix to tell your variables from the rest of the environment,
// without one it fails with ErrNoPrefix.
func OnUnknown(fn func(Unknown) error) Option {
	return func(o *options) { o.onUnknown = fn }
}

// Strict is OnUnknown with a handler that ends the parse, naming every key it could
// not place, with an error wrapping ErrUnknownField:
//
//	cnfg.Decode[Config](yaml.Unmarshal, "/etc/app/config.yaml", cnfg.Strict)
//	cnfg.Env[Config]("APP", cnfg.Strict)
var Strict = OnUnknown(func(u Unknown) error {
	return fmt.Errorf("%s: %w %s", u.Source, ErrUnknownField, strings.Join(u.Keys, ", "))
})

func newOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// report hands the keys to the handler, if there is one and there is anything to report.
func (o options) report(source string, keys []string) error {
	if o.onUnknown == nil || len(keys) == 0 {
		return nil
	}
	return o.onUnknown(Unknown{Source: source, Keys: keys})
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

	slices.Sort(unknown)
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
