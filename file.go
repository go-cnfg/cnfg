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

// Decode decodes the config file at path with dec on top of the config.
// encoding/json, go.yaml.in/yaml/v3 and github.com/BurntSushi/toml all provide
// an Unmarshal that can be used as dec, and so does anything with that signature.
func Decode(dec Decoder, path string, opts ...Option) Parser {
	o := newOptions(opts)
	return func(cfg any) error {
		return decodeFile(cfg, dec, path, o)
	}
}

// DecodeGlob decodes every file matching pattern with dec on top of the config, in the
// order filepath.Glob returns them, which is lexical. It is the drop-in directory
// convention of /etc, as in DecodeGlob[Config](yaml.Unmarshal, "/etc/app/config.d/*.conf"):
// a file later in the listing wins, and a pattern that matches nothing is a no op,
// so name the files 10-base.conf, 20-app.conf, 99-local.conf.
func DecodeGlob(dec Decoder, pattern string, opts ...Option) Parser {
	o := newOptions(opts)
	return func(cfg any) error {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("%w %s: %w", ErrReadFile, pattern, err)
		}

		for _, path := range paths {
			if info, err := os.Stat(path); err != nil || info.IsDir() {
				continue
			}
			if err := decodeFile(cfg, dec, path, o); err != nil {
				return err
			}
		}
		return nil
	}
}

// DecodeDir decodes every .conf file in dir with dec on top of the config, which is the
// drop-in directory convention of /etc. It is DecodeGlob with the usual pattern, so use
// that one directly when the files are named something else.
func DecodeDir(dec Decoder, dir string, opts ...Option) Parser {
	return DecodeGlob(dec, filepath.Join(dir, "*.conf"), opts...)
}

// Optional turns a missing config file into a no op.
func Optional(p Parser) Parser {
	return func(cfg any) error {
		if err := p(cfg); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
}

// DecodeFlag decodes the config file the user gave with the named flag, for example -config app.json.
// The flag is registered in set, which should be the one given to FlagSet later on, and args are
// scanned for it before any other source is read. It is a no op when the flag was not given.
func DecodeFlag(dec Decoder, set *flag.FlagSet, name string, args []string, opts ...Option) Parser {
	set.String(name, "", "path to config file")
	o := newOptions(opts)

	return func(cfg any) error {
		ff, err := configFields(cfg)
		if err != nil {
			return err
		}

		path := pathFromArgs(set.Name(), name, args, ff)
		if path == "" {
			return nil
		}
		return decodeFile(cfg, dec, path, o)
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

func decodeFile(cfg any, dec Decoder, path string, o options) error {
	v := configValue(cfg)
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("%w, got %T", ErrNotStruct, cfg)
	}

	data, err := os.ReadFile(path)
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

	if o.onUnknown != nil {
		return o.report(path, unknownKeys(v, tree, ""))
	}
	return nil
}
