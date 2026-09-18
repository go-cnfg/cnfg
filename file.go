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
// not there is a no op, so an optional config file needs no guard of its own. A key the
// file leaves out keeps the value the config already had.
func File(dec Decoder, path string) Source {
	return file(dec, path, false)
}

// FileStrict is [File] that also fails with [ErrUnknownField] on a key the config has no
// field for, which catches a typo in the file. Keys under a map or an any field are
// values rather than names, so they are left alone.
func FileStrict(dec Decoder, path string) Source {
	return file(dec, path, true)
}

func file(dec Decoder, path string, strict bool) Source {
	return sourceFunc(func(v reflect.Value) error {
		return readFile(v, dec, path, strict)
	})
}

// Glob decodes every file matching pattern with dec on top of the config, in the lexical
// order [filepath.Glob] returns them, so a later file wins. A pattern that matches
// nothing is a no op. It is the drop-in directory of /etc:
//
//	cnfg.Glob(yaml.Unmarshal, "/etc/app/config.d/*.conf")
//
// Name the files 10-base.conf, 20-app.conf, 99-local.conf to put them in the order you
// want them read.
func Glob(dec Decoder, pattern string) Source {
	return glob(dec, pattern, false)
}

// GlobStrict is [Glob] that also fails with [ErrUnknownField] on a key the config has no
// field for, naming the file that carries it.
func GlobStrict(dec Decoder, pattern string) Source {
	return glob(dec, pattern, true)
}

func glob(dec Decoder, pattern string, strict bool) Source {
	return sourceFunc(func(v reflect.Value) error {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("%w %s: %w", ErrReadFile, pattern, err)
		}

		for _, path := range paths {
			if info, err := os.Stat(path); err != nil || info.IsDir() {
				continue
			}
			if err := readFile(v, dec, path, strict); err != nil {
				return err
			}
		}
		return nil
	})
}

// FileFromFlag decodes the config file the user named with a flag, -config app.json. The
// flag is registered in set, which is the set to hand to [FlagSet] later, and args are
// scanned for it before any other source runs, so put this first. A flag that was not
// given, or that names a file which is not there, is a no op.
func FileFromFlag(dec Decoder, set *flag.FlagSet, name string, args []string) Source {
	return fileFromFlag(dec, set, name, args, false)
}

// FileFromFlagStrict is [FileFromFlag] that also fails with [ErrUnknownField] on a key
// the config has no field for.
func FileFromFlagStrict(dec Decoder, set *flag.FlagSet, name string, args []string) Source {
	return fileFromFlag(dec, set, name, args, true)
}

func fileFromFlag(dec Decoder, set *flag.FlagSet, name string, args []string, strict bool) Source {
	set.String(name, "", "path to config file")

	return sourceFunc(func(v reflect.Value) error {
		ff, err := fields(v)
		if err != nil {
			return err
		}

		path := pathFromArgs(set.Name(), name, args, ff)
		if path == "" {
			return nil
		}
		return readFile(v, dec, path, strict)
	})
}

// FileFromEnv decodes the config file named by an environment variable,
// APP_CONFIG=/etc/app/config.json. A variable that is unset or empty, or that names a
// file which is not there, is a no op. Next to an [EnvStrict] source, name the variable
// outside that prefix, which would read it as a typo.
func FileFromEnv(dec Decoder, name string) Source {
	return fileFromEnv(dec, name, false)
}

// FileFromEnvStrict is [FileFromEnv] that also fails with [ErrUnknownField] on a key the
// config has no field for.
func FileFromEnvStrict(dec Decoder, name string) Source {
	return fileFromEnv(dec, name, true)
}

func fileFromEnv(dec Decoder, name string, strict bool) Source {
	return sourceFunc(func(v reflect.Value) error {
		path := os.Getenv(name)
		if path == "" {
			return nil
		}
		return readFile(v, dec, path, strict)
	})
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

func readFile(v reflect.Value, dec Decoder, path string, strict bool) error {
	data, err := os.ReadFile(path) //nolint:gosec // reading the file the user named is the point
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w %s: %w", ErrReadFile, path, err)
	}

	tree := map[string]any{}
	if err := dec(data, &tree); err != nil {
		return fmt.Errorf("%w %s: %w", ErrDecodeFile, path, err)
	}
	if err := assignStruct(v, tree); err != nil {
		return fmt.Errorf("%w %s: %w", ErrDecodeFile, path, err)
	}

	if strict {
		return unknownField(path, unknownKeys(v, tree, ""))
	}
	return nil
}
