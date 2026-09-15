// Package yaml reads YAML config files with gopkg.in/yaml.v3.
package yaml

import (
	"flag"

	"github.com/go-cnfg/cnfg"
	"gopkg.in/yaml.v3"
)

// File decodes the YAML config file at path on top of the config.
func File[T any](path string) cnfg.Parser[T] {
	return cnfg.Decode[T](yaml.Unmarshal, path)
}

// FileFlag decodes the YAML config file the user gave with the named flag,
// for example -config app.yaml. See cnfg.DecodeFlag for the details.
func FileFlag[T any](set *flag.FlagSet, name string, args []string) cnfg.Parser[T] {
	return cnfg.DecodeFlag[T](yaml.Unmarshal, set, name, args)
}
