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
	"strings"
)

// File decodes the config file at path on top of the config.
// The decoder is picked from Decoders by the file extension.
func File[T any](path string) Parser[T] {
	return func(cfg T) (T, error) {
		return decodeFile(cfg, path)
	}
}

// Optional turns a missing config file into a no op.
func Optional[T any](p Parser[T]) Parser[T] {
	return func(cfg T) (T, error) {
		out, err := p(cfg)
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return out, err
	}
}

// FileFlag decodes the config file the user gave with the named flag, for example -config app.json.
// The flag is registered in set, which should be the one given to FlagSet later on, and args are
// scanned for it before any other source is read. It is a no op when the flag was not given.
func FileFlag[T any](set *flag.FlagSet, name string, args []string) Parser[T] {
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
		return decodeFile(cfg, path)
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

func decodeFile[T any](cfg T, path string) (T, error) {
	v := reflect.ValueOf(&cfg).Elem()
	if v.Kind() != reflect.Struct {
		return cfg, fmt.Errorf("%w, got %T", ErrNotStruct, cfg)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("%w %s: %w", ErrReadFile, path, err)
	}

	dec, ok := Decoders[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return cfg, fmt.Errorf("%w: %s", ErrUnknownFormat, path)
	}

	tree := map[string]any{}
	if err := dec(data, &tree); err != nil {
		return cfg, fmt.Errorf("%w %s: %w", ErrDecodeFile, path, err)
	}
	if err := assignStruct(v, tree); err != nil {
		return cfg, fmt.Errorf("%w %s: %w", ErrDecodeFile, path, err)
	}
	return cfg, nil
}
