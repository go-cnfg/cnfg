package cnfg

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
)

// File decodes the config file at path with dec on top of the config. A file that is
// not there is a no op, since a config file is the source that is allowed to be missing.
// encoding/json, go.yaml.in/yaml/v3 and github.com/BurntSushi/toml all provide an
// Unmarshal that can be used as dec, and so does anything with that signature.
func File[T any](dec Decoder, path string, opts ...Option) Parser[T] {
	o := newOptions(opts)
	return func(cfg T) (T, error) {
		return readFile(cfg, dec, path, o)
	}
}

// Glob decodes every file matching pattern with dec on top of the config, in the
// order filepath.Glob returns them, which is lexical. It is the drop-in directory
// convention of /etc, as in Glob[Config](yaml.Unmarshal, "/etc/app/config.d/*.conf"):
// a file later in the listing wins, and a pattern that matches nothing is a no op,
// so name the files 10-base.conf, 20-app.conf, 99-local.conf.
func Glob[T any](dec Decoder, pattern string, opts ...Option) Parser[T] {
	o := newOptions(opts)
	return func(cfg T) (T, error) {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			return cfg, fmt.Errorf("%w %s: %w", ErrReadFile, pattern, err)
		}

		for _, path := range paths {
			if info, err := os.Stat(path); err != nil || info.IsDir() {
				continue
			}
			if cfg, err = readFile(cfg, dec, path, o); err != nil {
				return cfg, err
			}
		}
		return cfg, nil
	}
}

// FileFlag decodes the config file the user gave with the named flag, for example -config app.json.
// The flag is registered in set, which should be the one given to FlagSet later on, and args are
// scanned for it before any other source is read. It is a no op when the flag was not given, or
// when it names a file that is not there.
func FileFlag[T any](dec Decoder, set *flag.FlagSet, name string, args []string, opts ...Option) Parser[T] {
	set.String(name, "", "path to config file")
	o := newOptions(opts)

	return func(cfg T) (T, error) {
		ff, err := configFields(&cfg)
		if err != nil {
			return cfg, err
		}

		path := pathFromArgs(set.Name(), name, args, ff)
		if path == "" {
			return cfg, nil
		}
		return readFile(cfg, dec, path, o)
	}
}

// pathFromArgs parses args with throwaway values to find out the config file path before
// any of the real values are set. Errors are left to the real flag parsing, which reports
// them with the proper usage output.
func pathFromArgs(setName, name string, args []string, ff []field) string {
	set := flag.NewFlagSet(setName, flag.ContinueOnError)
	set.SetOutput(io.Discard)

	path := set.String(name, "", "")
	for _, f := range ff {
		set.Var(discardValue(isBool(f.value.Type())), f.name, f.usage)
	}
	_ = set.Parse(args)

	return *path
}

func readFile[T any](cfg T, dec Decoder, path string, o options) (T, error) {
	v := reflect.ValueOf(&cfg).Elem()
	if v.Kind() != reflect.Struct {
		return cfg, fmt.Errorf("%w, got %T", ErrNotStruct, cfg)
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("%w %s: %w", ErrReadFile, path, err)
	}

	tree := map[string]any{}
	if err := dec(data, &tree); err != nil {
		return cfg, fmt.Errorf("%w %s: %w", ErrDecodeFile, path, err)
	}
	if err := assignStruct(v, tree); err != nil {
		return cfg, fmt.Errorf("%w %s: %w", ErrDecodeFile, path, err)
	}

	if o.onUnknown != nil {
		return cfg, o.report(path, unknownKeys(v, tree, ""))
	}
	return cfg, nil
}
