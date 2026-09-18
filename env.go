package cnfg

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

// Env reads the config from the environment. A field is named in upper case with
// underscores and the prefix in front, so field Addr of struct field Server is read
// from PREFIX_SERVER_ADDR, or from SERVER_ADDR when prefix is empty. A variable that
// is not set leaves its field alone.
//
// [EnvTag] renames a field here and nowhere else, `env:"LISTENADDR"`, for a variable
// that does not follow from the field name. Two fields that end up on the same variable
// fail with [ErrDuplicateName].
func Env(prefix string) Source {
	return sourceFunc(func(v reflect.Value) error {
		return envFrom(v, prefix, os.Environ(), false)
	})
}

// EnvStrict is [Env] that also fails with [ErrUnknownField] on a variable that carries
// the prefix but names no field, which catches a typo in a variable name. The prefix
// is what tells your variables from the rest of the environment, so an empty one fails
// with [ErrNoPrefix].
func EnvStrict(prefix string) Source {
	return sourceFunc(func(v reflect.Value) error {
		return envFrom(v, prefix, os.Environ(), true)
	})
}

// EnvFrom is [Env] over the given KEY=VALUE pairs instead of [os.Environ].
func EnvFrom(prefix string, environ []string) Source {
	return sourceFunc(func(v reflect.Value) error {
		return envFrom(v, prefix, environ, false)
	})
}

// EnvFromStrict is [EnvStrict] over the given KEY=VALUE pairs instead of [os.Environ].
func EnvFromStrict(prefix string, environ []string) Source {
	return sourceFunc(func(v reflect.Value) error {
		return envFrom(v, prefix, environ, true)
	})
}

func envFrom(v reflect.Value, prefix string, environ []string, strict bool) error {
	ff, err := fields(v)
	if err != nil {
		return err
	}

	env := make(map[string]string, len(environ))
	for _, e := range environ {
		if k, v, ok := strings.Cut(e, "="); ok {
			env[k] = v
		}
	}

	known := make(map[string]struct{}, len(ff))
	for _, f := range ff {
		key := envName(prefix, f.env)
		if _, ok := known[key]; ok {
			return fmt.Errorf("%w: %s", ErrDuplicateName, key)
		}
		known[key] = struct{}{}

		val, ok := env[key]
		if !ok {
			continue
		}
		if err := setValue(f.value, val); err != nil {
			return fmt.Errorf("%w for %s: %w", ErrInvalidValue, key, err)
		}
	}

	if strict {
		return unknownEnv(prefix, env, known)
	}
	return nil
}

// unknownEnv reports the first variable that carries the prefix but names no field.
func unknownEnv(prefix string, env map[string]string, known map[string]struct{}) error {
	p := envName(prefix, "")
	if p == "" {
		return ErrNoPrefix
	}

	var unknown []string
	for key := range env {
		if _, ok := known[key]; !ok && strings.HasPrefix(key, p) {
			unknown = append(unknown, key)
		}
	}
	return unknownField(prefix, unknown)
}

func envName(prefix, name string) string {
	if prefix = strings.Trim(prefix, "-_"); prefix != "" {
		name = prefix + "-" + name
	}
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}
