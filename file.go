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
func File[T any](dec Decoder, path string) Parser[T] {
	return file[T](dec, path, false)
}

// FileStrict is File that also fails on a key the config has no field for, with an error
// wrapping ErrUnknownField. Keys under a map or an any field are values rather than
// names, so they are left alone.
func FileStrict[T any](dec Decoder, path string) Parser[T] {
	return file[T](dec, path, true)
}

func file[T any](dec Decoder, path string, strict bool) Parser[T] {
	return func(cfg T) (T, error) {
		return readFile(cfg, dec, path, strict)
	}
}

// Glob decodes every file matching pattern with dec on top of the config, in the
// order filepath.Glob returns them, which is lexical. It is the drop-in directory
// convention of /etc, as in Glob[Config](yaml.Unmarshal, "/etc/app/config.d/*.conf"):
// a file later in the listing wins, and a pattern that matches nothing is a no op,
// so name the files 10-base.conf, 20-app.conf, 99-local.conf.
func Glob[T any](dec Decoder, pattern string) Parser[T] {
	return glob[T](dec, pattern, false)
}

// GlobStrict is Glob that also fails on a key the config has no field for, the way
// FileStrict does, naming the file that carries it.
func GlobStrict[T any](dec Decoder, pattern string) Parser[T] {
	return glob[T](dec, pattern, true)
}

func glob[T any](dec Decoder, pattern string, strict bool) Parser[T] {
	return func(cfg T) (T, error) {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			return cfg, fmt.Errorf("%w %s: %w", ErrReadFile, pattern, err)
		}

		for _, path := range paths {
			if info, err := os.Stat(path); err != nil || info.IsDir() {
				continue
			}
			if cfg, err = readFile(cfg, dec, path, strict); err != nil {
				return cfg, err
			}
		}
		return cfg, nil
	}
}

// FileFromFlag decodes the config file the user gave with the named flag, for example -config app.json.
// The flag is registered in set, which should be the one given to FlagSet later on, and args are
// scanned for it before any other source is read. It is a no op when the flag was not given, or
// when it names a file that is not there.
func FileFromFlag[T any](dec Decoder, set *flag.FlagSet, name string, args []string) Parser[T] {
	return fileFromFlag[T](dec, set, name, args, false)
}

// FileFromFlagStrict is FileFromFlag that also fails on a key the config has no field
// for, the way FileStrict does.
func FileFromFlagStrict[T any](dec Decoder, set *flag.FlagSet, name string, args []string) Parser[T] {
	return fileFromFlag[T](dec, set, name, args, true)
}

func fileFromFlag[T any](dec Decoder, set *flag.FlagSet, name string, args []string, strict bool) Parser[T] {
	set.String(name, "", "path to config file")

	return func(cfg T) (T, error) {
		ff, err := configFields(&cfg)
		if err != nil {
			return cfg, err
		}

		path := pathFromArgs(set.Name(), name, args, ff)
		if path == "" {
			return cfg, nil
		}
		return readFile(cfg, dec, path, strict)
	}
}

// FileFromEnv decodes the config file named by the environment variable, for example
// APP_CONFIG=/etc/app/config.json. It is a no op when the variable is not set or empty, or
// when it names a file that is not there. EnvStrict cannot tell the variable from a typo,
// so next to a strict env source name it outside that prefix.
func FileFromEnv[T any](dec Decoder, name string) Parser[T] {
	return fileFromEnv[T](dec, name, false)
}

// FileFromEnvStrict is FileFromEnv that also fails on a key the config has no field for,
// the way FileStrict does.
func FileFromEnvStrict[T any](dec Decoder, name string) Parser[T] {
	return fileFromEnv[T](dec, name, true)
}

func fileFromEnv[T any](dec Decoder, name string, strict bool) Parser[T] {
	return func(cfg T) (T, error) {
		path := os.Getenv(name)
		if path == "" {
			return cfg, nil
		}
		return readFile(cfg, dec, path, strict)
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

func readFile[T any](cfg T, dec Decoder, path string, strict bool) (T, error) {
	v := reflect.ValueOf(&cfg).Elem()
	if v.Kind() != reflect.Struct {
		return cfg, fmt.Errorf("%w, got %T", ErrNotStruct, cfg)
	}

	data, err := os.ReadFile(path) //nolint:gosec // reading the file the user named is the point
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

	if strict {
		return cfg, unknownField(path, unknownKeys(v, tree, ""))
	}
	return cfg, nil
}
