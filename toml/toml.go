// Package toml reads TOML config files with github.com/BurntSushi/toml.
package toml

import (
	"flag"

	"github.com/BurntSushi/toml"
	"github.com/go-cnfg/cnfg"
)

// File decodes the TOML config file at path on top of the config.
func File[T any](path string) cnfg.Parser[T] {
	return cnfg.Decode[T](toml.Unmarshal, path)
}

// FileFlag decodes the TOML config file the user gave with the named flag,
// for example -config app.toml. See cnfg.DecodeFlag for the details.
func FileFlag[T any](set *flag.FlagSet, name string, args []string) cnfg.Parser[T] {
	return cnfg.DecodeFlag[T](toml.Unmarshal, set, name, args)
}
