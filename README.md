[![Go Reference](https://pkg.go.dev/badge/github.com/go-cnfg/cnfg.svg)](https://pkg.go.dev/github.com/go-cnfg/cnfg) ![main](https://github.com/go-cnfg/cnfg/actions/workflows/go.yaml/badge.svg?branch=main) [![codecov](https://codecov.io/gh/go-cnfg/cnfg/branch/main/graph/badge.svg)](https://codecov.io/gh/go-cnfg/cnfg)

# cnfg

Dead simple zero dependency config parser.

You declare your config as a plain struct, fill it with the defaults you want and hand it to
`cnfg.Parse` together with the sources you want to read. Sources are applied in the order you
give them, so the last one wins.

```go
import (
    "encoding/json"
    "log"
    "time"

    "github.com/go-cnfg/cnfg"
)

type Config struct {
    Addr    string        `usage:"address to listen on"`
    Timeout time.Duration `usage:"request timeout"`
    Debug   bool          `usage:"enable debug logging"`
}

func main() {
    cfg, err := cnfg.Parse(Config{
        Addr:    ":8080",
        Timeout: 5 * time.Second,
    },
        cnfg.File(json.Unmarshal, "/etc/app/config.json"),
        cnfg.Env("APP"),
        cnfg.Flags(),
    )
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("%+v", cfg)
}
```

## Table of Contents

- [Install](#install)
- [Sources](#sources)
- [Names](#names)
- [Tags](#tags)
- [Config files](#config-files)
- [Drop-in directories](#drop-in-directories)
- [Extra keys](#extra-keys)
- [Types](#types)
- [Validation](#validation)
- [Flags](#flags)
- [Exiting on error](#exiting-on-error)

## Install

```sh
go get github.com/go-cnfg/cnfg
```

cnfg reads env vars, flags and config files and needs nothing outside the standard library. A
file format is a decoder you pass in, so the format library stays a dependency of your own
program and cnfg never drags one in.

Struct tag validation lives in [github.com/go-cnfg/validator](https://github.com/go-cnfg/validator),
a module of its own that wraps go-playground/validator as a source.

## Sources

A source reads one place your config can come from. `Env`, `Flags` and the file sources all
return a `cnfg.Source`, and the config type is inferred from the defaults you hand to `Parse`,
so no source needs a type argument:

```go
cfg, err := cnfg.Parse(Config{Addr: ":8080"}, cnfg.Env("APP"), cnfg.Flags())
```

Your own validation or post processing fits in the same chain, wrapped in `cnfg.Func`:

```go
func normalize(cfg Config) (Config, error) {
    if cfg.Addr == "" {
        return cfg, errors.New("addr is required")
    }
    cfg.Addr = strings.TrimPrefix(cfg.Addr, "http://")
    return cfg, nil
}

cfg, err := cnfg.Parse(Config{Addr: ":8080"}, cnfg.Env("APP"), cnfg.Flags(), cnfg.Func(normalize))
```

Fields that no source touched keep the value they had in defaults, so there is no need to
repeat the defaults in a file or to guard against empty values. `Parse` returns the zero value
of your config together with the error when a source fails.

| Source | Reads |
| --- | --- |
| `Env(prefix)` | Environment variables, from `os.Environ()`. |
| `EnvFrom(prefix, environ)` | Environment variables, from the given `KEY=VALUE` list. |
| `Flags()` | Command line flags, from `os.Args[1:]`. |
| `FlagSet(set, args)` | Command line flags, from your own flag set and args. |
| `File(dec, path)` | Config file at path, decoded with dec. |
| `Glob(dec, pattern)` | Every file matching the glob, in lexical order. |
| `FileFromFlag(dec, name, args)` | Config file the user gave with a flag, `-config app.json`. |
| `FileFromEnv(dec, name)` | Config file named by an env var, `APP_CONFIG=app.json`. |
| `Func(fn)` | Whatever your own `func(T) (T, error)` does. |

Each of `Env`, `EnvFrom` and the file sources has a `Strict` twin, `EnvStrict`, `FileStrict` and
so on, that fails on a key your config has no field for, see [extra keys](#extra-keys).

Nothing is magic about the order. Put the sources in the order you want them to win, and put
your own among them wherever they belong.

## Names

The name of a field is derived from the Go field name, `ListenAddr` becomes `listen-addr`.
Nested structs are joined with a dash, env vars use the same name in upper case with underscores.

```go
type Config struct {
    ListenAddr string // -listen-addr, LISTEN_ADDR
    Server     struct {
        TLSCert string // -server-tls-cert, SERVER_TLS_CERT
    }
}
```

A variable that does not follow from the field name gets an `env` tag, which renames the field
in the environment sources and nowhere else:

```go
type Config struct {
    ListenAddr  string `env:"LISTENADDR"` // -listen-addr, APP_LISTENADDR
    DatabaseURL string `env:"DB_URL"`     // -database-url, APP_DB_URL
}
```

The prefix is still put in front and nested fields are still joined, so the tag replaces one
segment of the name rather than the whole of it. It is normalised like any other name, so
`env:"listen-addr"` and `env:"LISTEN_ADDR"` are the same variable, and two fields that land on
the same variable fail with `ErrDuplicateName`.

In config files both the generated name and the Go field name are accepted, and matching
ignores case, dashes and underscores. `{"server": {"tls-cert": "a.pem"}}` and
`{"Server": {"TLSCert": "a.pem"}}` do the same thing.

## Tags

| Tag | Purpose |
| --- | --- |
| `cnfg:"name"` | Use the given name instead of the generated one. |
| `cnfg:"-"` | Leave the field out of all sources. |
| `usage:"text"` | Document the field in the usage output. |
| `env:"NAME"` | Use the given name in the environment sources only. |
| `cnfg:",require"` | Make `Require` fail when the field is still zero, after a name or on its own. |

## Config files

`File` takes the function that decodes the bytes into a `*map[string]any`, which is what
`encoding/json`, `go.yaml.in/yaml/v3` and `github.com/BurntSushi/toml` all give you, so the
format is simply the library you already import:

```go
import "go.yaml.in/yaml/v3"

cfg, err := cnfg.Parse(Config{Addr: ":8080"}, cnfg.File(yaml.Unmarshal, "/etc/app/config.yaml"))
```

A config file that is not there is a no op, since that is the one source allowed to be
missing, and your defaults are the config in that case. A file that is there but cannot be
read or decoded is still an error, wrapping `ErrReadFile` or `ErrDecodeFile`.

Anything with that signature works, so a format cnfg has never heard of needs no support from
cnfg:

```go
cfg, err := cnfg.Parse(Config{Addr: ":8080"}, cnfg.File(hcl.Unmarshal, "/etc/app/config.hcl"))
```

Values from files go through the same parsing as env vars and flags, so a duration is written
as `"30s"` and a `net.IP` as `"10.0.0.1"` in every source.

To let the user point at a config file with a flag, put `FileFromFlag` first. It scans the args
for the flag on its own, so the file is read before the other sources even though it was named on
the command line, and `Parse` registers the flag in the set of the `Flags` or `FlagSet` in the
same call, so it is listed in the usage output like any other:

```go
cfg, err := cnfg.Parse(Config{Addr: ":8080"},
    cnfg.FileFromFlag(json.Unmarshal, "config", os.Args[1:]),
    cnfg.Env("APP"),
    cnfg.Flags(),
)
```

`FileFromEnv` does the same for an environment variable, and is a no op when the variable is
empty or names a file that is not there:

```go
cfg, err := cnfg.Parse(Config{Addr: ":8080"},
    cnfg.FileFromEnv(json.Unmarshal, "APP_CONFIG"),
    cnfg.Env("APP"),
    cnfg.Flags(),
)
```

`EnvStrict` cannot tell that variable from a typo, so next to a strict env source name it
outside the prefix, `CONFIG_FILE` rather than `APP_CONFIG`.

## Drop-in directories

`Glob` reads a whole `conf.d` directory the way the rest of `/etc` does: every file the
pattern matches, in lexical order, each one on top of the last. A directory that is not there
is a no op like a missing file, so the usual base file plus drop-ins looks like this:

```go
import "go.yaml.in/yaml/v3"

cfg, err := cnfg.Parse(Config{Addr: ":8080"},
    cnfg.File(yaml.Unmarshal, "/etc/app/config.yaml"),
    cnfg.Glob(yaml.Unmarshal, "/etc/app/config.d/*.conf"),
    cnfg.Env("APP"),
    cnfg.Flags(),
)
```

```
/etc/app/config.d/10-base.conf
/etc/app/config.d/50-limits.conf
/etc/app/config.d/99-local.conf
```

Number the files the way sysctl.d and systemd drop-ins do, since the last one to set a field
wins. `.conf` is what `/etc` uses, but the pattern is yours, so `*.yaml` works just as well.

## Extra keys

A key a config file has but your config does not is ignored, which is friendly to a file that
several programs read and unfriendly to a typo. Every source that can have extra keys has a
`Strict` twin that fails on anything it cannot place: `FileStrict`, `GlobStrict`,
`FileFromFlagStrict`, `FileFromEnvStrict`, `EnvStrict` and `EnvFromStrict`.

```go
cfg, err := cnfg.Parse(Config{Addr: ":8080"},
    cnfg.FileStrict(yaml.Unmarshal, "/etc/app/config.yaml"),
    cnfg.EnvStrict("APP"),
    cnfg.Flags(),
)
```

```
/etc/app/config.yaml: no field for server.tls-cret
APP: no field for APP_ADRR
```

The error wraps `ErrUnknownField` and names the first key it could not place, nested ones
joined with a dot and slice elements with their index. `GlobStrict` names the file that
carries the key, so a drop-in directory points at the right file. Keys under a `map` or an
`any` field are values rather than names, so they are left alone. Flags are strict on their
own, an unknown flag has always been an error.

For env vars the prefix is what tells yours from the rest of the environment, so `APP_ADRR` is
reported while `PATH` is not, and `EnvStrict` without a prefix fails with `ErrNoPrefix` rather
than reading the whole environment as yours.

## Types

Anything that can be built from a single string works: strings, bools, ints, uints, floats,
`time.Duration`, `[]byte` and every type implementing `encoding.TextUnmarshaler` or `flag.Value`,
like `net.IP` and `time.Time`. Slices of those are comma separated on the command line and in
env vars, `-hosts a,b,c`.

Maps and slices of structs can only be filled from a config file since there is no sane way to
express them as a flag. They are skipped by the flag and env sources.

## Validation

A value that does not fit the field is rejected as it is read, so `WORKERS=many` on an `int`,
a duration `time.ParseDuration` will not take, or anything your own `Set` or `UnmarshalText`
turns down ends the parse with an error wrapping `ErrInvalidValue`:

```
invalid value for APP_WORKERS: strconv.ParseInt: parsing "many": invalid syntax
```

A flag that will not parse is reported by the flag package itself, so that one wraps
`ErrParseFlags` instead.

Beyond that cnfg ships one validator, `Require`. A field that has to be set gets `require` in
its `cnfg` tag, and `Require` fails on the first one that still has its zero value once the
sources before it have been read:

```go
type Config struct {
    Addr    string `cnfg:",require"`
    Workers int
}

cfg, err := cnfg.Parse(Config{Workers: 4}, cnfg.Env("APP"), cnfg.Flags(), cnfg.Require())
```

```
required field not set: addr
```

The error wraps `ErrRequired` and names the field the way the flag does, `server-addr` for
field `Addr` of struct field `Server`. The zero value is what counts as not set, so a bool or a
number that may well be zero is not something to require.

Anything more than that is deliberately not built in. A validator is a source like any other,
so a plain function or whatever struct validator you already use fits at the end of the chain:

```go
func validate(cfg Config) (Config, error) {
    if cfg.Workers < 1 {
        return cfg, fmt.Errorf("workers must be at least 1, got %d", cfg.Workers)
    }
    return cfg, nil
}

cfg, err := cnfg.Parse(Config{Workers: 4}, cnfg.Env("APP"), cnfg.Flags(), cnfg.Func(validate))
```

[github.com/go-cnfg/validator](https://github.com/go-cnfg/validator) is one such wrapper, it
runs [go-playground/validator](https://github.com/go-playground/validator) as a source so the
rules live in struct tags:

```go
import "github.com/go-cnfg/validator"

type Config struct {
    Addr    string        `validate:"required,hostname_port"`
    Workers int           `validate:"gte=1,lte=100"`
    Level   string        `validate:"oneof=debug info warn"`
    Timeout time.Duration `validate:"min=1s"`
}

cfg, err := cnfg.Parse(Config{
    Addr:    ":8080",
    Workers: 4,
    Level:   "info",
    Timeout: 30 * time.Second,
},
    cnfg.Env("APP"),
    cnfg.Flags(),
    validator.Validate(),
)
```

Put the check last so every source has been read, and remember that the parse stops there:
`Parse` returns the zero value of your config together with the error.

## Flags

Flags are registered in a normal `flag.FlagSet`, so `-h` prints the usual listing with the types
and defaults from your struct:

```
$ app -h
Usage of app:
  -addr string
    	address to listen on (default :8080)
  -timeout duration
    	request timeout (default 5s)
  -debug
    	enable debug logging
```

`Parse` returns an error wrapping `flag.ErrHelp` when the user asked for the usage output, which
is the normal way to exit with status 0:

```go
cfg, err := cnfg.Parse(Config{Addr: ":8080"}, cnfg.Flags())
if errors.Is(err, flag.ErrHelp) {
    return
}
```

A `main` with nowhere to put that error can leave it to `MustParse`, see
[exiting on error](#exiting-on-error).

Use `FlagSet` when you want to name the flag set, write the usage somewhere else, register flags
of your own or read the positional args that were left over:

```go
set := flag.NewFlagSet("app", flag.ContinueOnError)

cfg, err := cnfg.Parse(Config{Addr: ":8080"}, cnfg.FlagSet(set, os.Args[1:]))
if err != nil {
    log.Fatal(err)
}

log.Println(set.Args())
```

Give the set `flag.ContinueOnError` so a bad flag and `-h` come back from `Parse` as errors.
With `flag.ExitOnError`, which is what `flag.CommandLine` uses, the flag package exits the
process from inside `Parse` instead, status 2 for a bad flag and 0 for `-h`, and the
`flag.ErrHelp` check above never runs. `Flags` makes its set with `flag.ContinueOnError` for
this reason:

```go
set := flag.NewFlagSet("app", flag.ContinueOnError)

_, err := cnfg.Parse(Config{Addr: ":8080"}, cnfg.FlagSet(set, []string{"-prot", "9090"}))
fmt.Println(err)
fmt.Println(errors.Is(err, cnfg.ErrParseFlags))
```

```
failed to parse flags: flag provided but not defined: -prot
true
```

`Parse` leaves that choice alone, so the set keeps whatever it was made with. `MustParse` does
not, see [exiting on error](#exiting-on-error).

Flag parsing is a source like any other, so a library with a different flavor of flags takes the
same place in the chain. With [go-flags](https://github.com/jessevdk/go-flags) the flags come
from its own struct tags, and `cnfg.Func` wraps the parse:

```go
import "github.com/jessevdk/go-flags"

type Config struct {
    Addr    string        `long:"addr" description:"address to listen on"`
    Timeout time.Duration `long:"timeout" description:"request timeout"`
    Debug   bool          `long:"debug" description:"enable debug logging"`
}

func ParseFlags(cfg Config) (Config, error) {
    _, err := flags.ParseArgs(&cfg, os.Args[1:])
    if flags.WroteHelp(err) {
        return cfg, flag.ErrHelp
    }
    return cfg, err
}

cfg, err := cnfg.Parse(Config{Addr: ":8080"},
    cnfg.File(json.Unmarshal, "/etc/app/config.json"),
    cnfg.Env("APP"),
    cnfg.Func(ParseFlags),
)
if errors.Is(err, flag.ErrHelp) {
    return
}
```

go-flags leaves a field alone when its flag was not given, so the values from the file and the
env vars come through the way they do with `Flags`. It has its own way of saying `--help` was
asked for, and turning that into `flag.ErrHelp` inside the function keeps the rest of the program
the same as with `Flags`.

## Exiting on error

`MustParse` is `Parse` for a `main` that has nowhere to put an error. It gives the config when
every source read, and otherwise writes the error to stderr and exits with status 1:

```go
func main() {
    cfg := cnfg.MustParse(Config{},
        cnfg.File(json.Unmarshal, "/etc/app/config.json"),
        cnfg.Env("APP"),
        cnfg.Flags(),
        cnfg.Require(),
    )

    log.Printf("%+v", cfg)
}
```

```
$ app
required field not set: addr
$ echo $?
1
```

Two errors it adds nothing of its own to, since they are on the screen by the time it sees them.
`-h` is the user asking for the usage output rather than a failure, so the flag set writes the
listing and `MustParse` exits with status 0. A flag that will not parse is reported by the flag
set as well, together with the usage, and that one exits with status 1.

### The flag set is switched to ContinueOnError

The status is `MustParse`'s to pick, so it switches every flag set it is given to
`flag.ContinueOnError` before reading it. The set keeps that setting after the call, so this is
a change to an object you own, not only to how `MustParse` reads it.

```go
set := flag.NewFlagSet("app", flag.ExitOnError)

cfg := cnfg.MustParse(Config{}, cnfg.FlagSet(set, os.Args[1:]))

// set.ErrorHandling() is flag.ContinueOnError from here on
```

Without it a set made with `flag.ExitOnError` would end the program from inside the flag
package, with status 2 for a flag it could not parse, and one made with `flag.PanicOnError`
would panic. Either way `MustParse` would never get to pick.

`Parse` does none of this. A set handed to `Parse` keeps the error handling it was made with,
which is why the [Flags](#flags) section asks you to pick `flag.ContinueOnError` yourself
there.

Use `Parse` where the caller has somewhere better to put the error, a library, a test, or a
`main` that logs it its own way.
