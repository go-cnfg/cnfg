// Package validator checks the parsed config with github.com/go-playground/validator.
package validator

import (
	"github.com/go-cnfg/cnfg"
	"github.com/go-playground/validator/v10"
)

var std = validator.New()

// Validate checks the config against the validate tags of its fields.
// Add it as the last parser, once all the sources have been read:
//
//	type Config struct {
//		Addr    string `validate:"required,hostname_port"`
//		Workers int    `validate:"gte=1,lte=100"`
//	}
//
//	cfg, err := cnfg.Parse(defaults, cnfg.Flags[Config](), validator.Validate[Config]())
func Validate[T any]() cnfg.Parser[T] {
	return With[T](std)
}

// With works like Validate but uses the given validator, for the custom rules,
// tag name or translations you set up yourself.
func With[T any](v *validator.Validate) cnfg.Parser[T] {
	return func(cfg T) (T, error) {
		return cfg, v.Struct(cfg)
	}
}
