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
		return envFrom(cfg, prefix, os.Environ())
	}
}

// EnvFrom works like Env but reads the given KEY=VALUE pairs instead of os.Environ().
func EnvFrom[T any](prefix string, environ []string) Parser[T] {
	return func(cfg T) (T, error) {
		return envFrom(cfg, prefix, environ)
	}
}

func envFrom[T any](cfg T, prefix string, environ []string) (T, error) {
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

	for _, f := range ff {
		key := envName(prefix, f.name)
		val, ok := env[key]
		if !ok {
			continue
		}
		if err := setValue(f.value, val); err != nil {
			return cfg, fmt.Errorf("%w for %s: %w", ErrInvalidValue, key, err)
		}
	}
	return cfg, nil
}

func envName(prefix, name string) string {
	if prefix = strings.Trim(prefix, "-_"); prefix != "" {
		name = prefix + "-" + name
	}
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}
