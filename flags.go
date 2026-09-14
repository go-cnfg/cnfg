package cnfg

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Flags parses os.Args[1:] into the config.
func Flags[T any]() Parser[T] {
	return FlagSet[T](flag.NewFlagSet(filepath.Base(os.Args[0]), flag.ContinueOnError), os.Args[1:])
}

// FlagSet parses args with the given flag set. Flags that were registered before are left alone,
// and the positional args are available with set.Args() once Parse is done.
// The error wraps ErrParseFlags, and flag.ErrHelp when the user asked for the usage output.
func FlagSet[T any](set *flag.FlagSet, args []string) Parser[T] {
	return func(cfg T) (T, error) {
		ff, err := configFields(&cfg)
		if err != nil {
			return cfg, err
		}

		set.Usage = usageFunc(set, ff)
		for _, f := range ff {
			set.Var(&flagValue{value: f.value}, f.name, f.usage)
		}

		if err := set.Parse(args); err != nil {
			return cfg, fmt.Errorf("%w: %w", ErrParseFlags, err)
		}
		return cfg, nil
	}
}

// usageFunc lists the flags with their type and default value. Both are read before the
// fields are registered, so the output shows the defaults instead of the parsed values.
func usageFunc(set *flag.FlagSet, ff []field) func() {
	var own []*flag.Flag
	set.VisitAll(func(f *flag.Flag) { own = append(own, f) })

	defs := make([]string, len(ff))
	for i, f := range ff {
		defs[i] = stringOf(f.value)
	}

	return func() {
		b := &strings.Builder{}
		if set.Name() == "" {
			b.WriteString("Usage:\n")
		} else {
			fmt.Fprintf(b, "Usage of %s:\n", set.Name())
		}

		for _, f := range own {
			typ, usage := flag.UnquoteUsage(f)
			writeFlag(b, f.Name, typ, usage, ownDefault(f))
		}
		for i, f := range ff {
			writeFlag(b, f.name, typeName(f.value.Type()), f.usage, fieldDefault(f, defs[i]))
		}

		_, _ = io.WriteString(set.Output(), b.String())
	}
}

func writeFlag(b *strings.Builder, name, typ, usage, def string) {
	fmt.Fprintf(b, "  -%s", name)
	if typ != "" {
		fmt.Fprintf(b, " %s", typ)
	}

	if def != "" {
		if usage != "" {
			usage += " "
		}
		usage += fmt.Sprintf("(default %s)", def)
	}
	if usage != "" {
		fmt.Fprintf(b, "\n    \t%s", usage)
	}
	b.WriteString("\n")
}

func ownDefault(f *flag.Flag) string {
	if b, ok := f.Value.(boolFlag); ok && b.IsBoolFlag() && f.DefValue == "false" {
		return ""
	}
	return f.DefValue
}

func fieldDefault(f field, def string) string {
	if isBool(f.value.Type()) && def == "false" {
		return ""
	}
	return def
}

type boolFlag interface {
	IsBoolFlag() bool
}
