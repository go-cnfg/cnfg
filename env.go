package cnfg

import (
	"fmt"
	"os"
	"strings"
)

// Env reads the environment variables named after the config fields, prefixed with prefix.
// Field Addr of struct field Server is read from PREFIX_SERVER_ADDR, or from SERVER_ADDR
// when the prefix is empty.
func Env[T any](prefix string) Parser[T] {
	return func(cfg T) (T, error) {
		return envFrom(cfg, prefix, os.Environ(), false)
	}
}

// EnvStrict is Env that also fails on a variable that carries the prefix but names no
// field, with an error wrapping ErrUnknownField. The prefix is what tells your variables
// from the rest of the environment, so an empty one fails with ErrNoPrefix.
func EnvStrict[T any](prefix string) Parser[T] {
	return func(cfg T) (T, error) {
		return envFrom(cfg, prefix, os.Environ(), true)
	}
}

// EnvFrom works like Env but reads the given KEY=VALUE pairs instead of os.Environ().
func EnvFrom[T any](prefix string, environ []string) Parser[T] {
	return func(cfg T) (T, error) {
		return envFrom(cfg, prefix, environ, false)
	}
}

// EnvFromStrict works like EnvStrict but reads the given KEY=VALUE pairs instead of os.Environ().
func EnvFromStrict[T any](prefix string, environ []string) Parser[T] {
	return func(cfg T) (T, error) {
		return envFrom(cfg, prefix, environ, true)
	}
}

func envFrom[T any](cfg T, prefix string, environ []string, strict bool) (T, error) {
	ff, err := configFields(&cfg)
	if err != nil {
		return cfg, err
	}

	env := make(map[string]string, len(environ))
	for _, e := range environ {
		if k, v, ok := strings.Cut(e, "="); ok {
			env[k] = v
		}
	}

	known := make(map[string]struct{}, len(ff))
	for _, f := range ff {
		key := envName(prefix, f.name)
		known[key] = struct{}{}

		val, ok := env[key]
		if !ok {
			continue
		}
		if err := setValue(f.value, val); err != nil {
			return cfg, fmt.Errorf("%w for %s: %w", ErrInvalidValue, key, err)
		}
	}

	if strict {
		return cfg, unknownEnv(prefix, env, known)
	}
	return cfg, nil
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
