// Package json reads JSON config files with encoding/json.
package json

import (
	"encoding/json"
	"flag"
	"path/filepath"

	"github.com/go-cnfg/cnfg"
)

// File decodes the JSON config file at path on top of the config.
func File[T any](path string) cnfg.Parser[T] {
	return cnfg.Decode[T](json.Unmarshal, path)
}

// FileFlag decodes the JSON config file the user gave with the named flag,
// for example -config app.json. See cnfg.DecodeFlag for the details.
func FileFlag[T any](set *flag.FlagSet, name string, args []string) cnfg.Parser[T] {
	return cnfg.DecodeFlag[T](json.Unmarshal, set, name, args)
}

// Dir decodes every .conf file in dir on top of the config, in lexical order, the way
// /etc drop-in directories work. A missing dir is a no op. See cnfg.DecodeGlob for a
// pattern of your own.
func Dir[T any](dir string) cnfg.Parser[T] {
	return cnfg.DecodeGlob[T](json.Unmarshal, filepath.Join(dir, "*.conf"))
}
