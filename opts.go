package cnfg

import (
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
)

// Option changes how a source is read.
type Option func(*options)

type options struct {
	onUnknown func(Unknown) error
}

// Unknown is one key a source could not place, with the value it carried and the file
// it was read from or the env prefix it was looked up with as the source.
type Unknown struct {
	Source string
	Key    string
	Value  any
}

// OnUnknown hands fn every key a source could not place, one call per key, so you can
// fail on them, log them or count them. The error it returns ends the parse, and the
// keys of one source come in order:
//
//	cnfg.Decode[Config](yaml.Unmarshal, path, cnfg.OnUnknown(func(u cnfg.Unknown) error {
//		log.Printf("%s: ignoring %s=%v", u.Source, u.Key, u.Value)
//		return nil
//	}))
//
// Keys under a map or an any field are values rather than names, so they are left
// alone. Env needs a prefix to tell your variables from the rest of the environment,
// without one it fails with ErrNoPrefix.
func OnUnknown(fn func(Unknown) error) Option {
	return func(o *options) { o.onUnknown = fn }
}

// Strict is OnUnknown with a handler that ends the parse on the first key it could not
// place, with an error wrapping ErrUnknownField:
//
//	cnfg.Decode[Config](yaml.Unmarshal, "/etc/app/config.yaml", cnfg.Strict)
//	cnfg.Env[Config]("APP", cnfg.Strict)
var Strict = OnUnknown(func(u Unknown) error {
	return fmt.Errorf("%s: %w %s", u.Source, ErrUnknownField, u.Key)
})

// LogUnknown is OnUnknown with a handler that logs every key it could not place with
// the default slog logger and lets the parse go on:
//
//	cnfg.Decode[Config](yaml.Unmarshal, "/etc/app/config.yaml", cnfg.LogUnknown)
//
// The value is left out of the log on purpose, a mistyped key can still carry a secret.
var LogUnknown = OnUnknown(func(u Unknown) error {
	slog.Warn("config key with no field", "source", u.Source, "key", u.Key)
	return nil
})

func newOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// report hands the keys to the handler one by one, in order, and stops at the first
// error it gives back. Callers check that there is a handler before they look for keys.
func (o options) report(source string, unknown []Unknown) error {
	slices.SortFunc(unknown, func(a, b Unknown) int { return strings.Compare(a.Key, b.Key) })
	for _, u := range unknown {
		u.Source = source
		if err := o.onUnknown(u); err != nil {
			return err
		}
	}
	return nil
}

// unknownKeys lists the keys of tree that no field of the struct behind v reads,
// with the names the file itself used, nested keys joined with a dot.
func unknownKeys(v reflect.Value, tree map[string]any, prefix string) []Unknown {
	fields := treeFields(v)

	var unknown []Unknown
	for key, x := range tree {
		fv, ok := fields[normalize(key)]
		if !ok {
			unknown = append(unknown, Unknown{Key: prefix + key, Value: x})
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
func unknownIn(v reflect.Value, x any, prefix string) []Unknown {
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

		var unknown []Unknown
		for i, item := range list {
			elem := reflect.New(v.Type().Elem()).Elem()
			unknown = append(unknown, unknownIn(elem, item, fmt.Sprintf("%s[%d].", strings.TrimSuffix(prefix, "."), i))...)
		}
		return unknown
	}
	return nil
}
