package cnfg

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

// field is a single leaf value of the config struct together with the name it is known by.
type field struct {
	name  string
	env   string
	usage string
	value reflect.Value
}

// tag holds what the cnfg struct tag says about a field.
type tag struct {
	name    string
	named   bool
	skip    bool
	require bool
}

// parseTag reads the cnfg tag of the field, for example `cnfg:"addr,require"`.
// The name falls back to the field name.
func parseTag(sf reflect.StructField) tag {
	name, opts, _ := strings.Cut(sf.Tag.Get(NameTag), ",")
	t := tag{name: name, named: name != ""}
	if name == "-" {
		t.skip = true
		return t
	}

	if !t.named {
		t.name = kebab(sf.Name)
	}
	for _, opt := range strings.Split(opts, ",") {
		if strings.TrimSpace(opt) == "require" {
			t.require = true
		}
	}
	return t
}

// fields collects every leaf value of the struct. Names of nested fields are joined with a dash,
// so field Addr of struct field Server is named server-addr.
// Values that can't be expressed as a single string, like maps and slices of structs,
// are left out since only the config file can fill them.
func fields(v reflect.Value) ([]field, error) {
	var ff []field
	collect(v, "", "", &ff)

	seen := make(map[string]struct{}, len(ff))
	for _, f := range ff {
		if _, ok := seen[f.name]; ok {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateName, f.name)
		}
		seen[f.name] = struct{}{}
	}
	return ff, nil
}

func collect(v reflect.Value, prefix, envPrefix string, ff *[]field) {
	t := v.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}

		tag := parseTag(sf)
		if tag.skip {
			continue
		}

		env := tag.name
		if alias := sf.Tag.Get(EnvTag); alias != "" {
			env = alias
		}

		fv := v.Field(i)
		if isLeaf(fv.Type()) {
			*ff = append(*ff, field{
				name:  prefix + tag.name,
				env:   envPrefix + env,
				usage: sf.Tag.Get(UsageTag),
				value: fv,
			})
			continue
		}

		sv, ok := structValue(fv)
		if !ok {
			continue
		}

		p, ep := prefix, envPrefix
		if !sf.Anonymous || tag.named {
			p = prefix + tag.name + "-"
			ep = envPrefix + env + "-"
		}
		collect(sv, p, ep, ff)
	}
}

// structValue returns the struct behind v, allocating it when v is a nil pointer.
func structValue(v reflect.Value) (reflect.Value, bool) {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	return v, v.Kind() == reflect.Struct
}

// kebab turns a Go field name into a flag name: ListenAddr becomes listen-addr and HTTPAddr http-addr.
func kebab(s string) string {
	b := strings.Builder{}
	rs := []rune(s)
	for i, r := range rs {
		if r == '_' {
			b.WriteRune('-')
			continue
		}
		if i > 0 && unicode.IsUpper(r) && startsWord(rs, i) {
			b.WriteRune('-')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// startsWord reports whether the upper case rune at i starts a new word,
// so that HTTPAddr is split only once.
func startsWord(rs []rune, i int) bool {
	return !unicode.IsUpper(rs[i-1]) || (i+1 < len(rs) && unicode.IsLower(rs[i+1]))
}
