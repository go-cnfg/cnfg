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
        cnfg.File[Config](json.Unmarshal, "/etc/app/config.json"),
        cnfg.Env[Config]("APP"),
        cnfg.Flags[Config](),
    )
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("%+v", cfg)
}
```

## Table of Contents

- [Install](#install)
- [Parsers](#parsers)
- [Names](#names)
- [Tags](#tags)
- [Config files](#config-files)
- [Drop-in directories](#drop-in-directories)
- [Extra keys](#extra-keys)
- [Types](#types)
- [Validation](#validation)
- [Flags](#flags)

## Install

```sh
go get github.com/go-cnfg/cnfg
```

cnfg reads env vars, flags and config files and needs nothing outside the standard library. A
file format is a decoder you pass in, so the format library stays a dependency of your own
program and cnfg never drags one in.

Struct tag validation lives in [github.com/go-cnfg/validator](https://github.com/go-cnfg/validator),
a module of its own that wraps go-playground/validator as a parser.

## Parsers

A parser is anything that takes a config and gives back a config:

```go
type Parser[T any] func(T) (T, error)
```

`Env`, `Flags` and the file parsers are parsers that happen to read a source, and your own
validation or post processing fits in the same chain:

```go
func normalize(cfg Config) (Config, error) {
    if cfg.Addr == "" {
        return cfg, errors.New("addr is required")
    }
    cfg.Addr = strings.TrimPrefix(cfg.Addr, "http://")
    return cfg, nil
}

cfg, err := cnfg.Parse(defaults, cnfg.Env[Config]("APP"), cnfg.Flags[Config](), normalize)
```

Fields that no parser touched keep the value they had in defaults, so there is no need to
repeat the defaults in a file or to guard against empty values. `Parse` returns the zero value
of your config together with the error when a parser fails.

| Parser | Reads |
| --- | --- |
| `Env[T](prefix)` | Environment variables, from `os.Environ()`. |
| `EnvFrom[T](prefix, environ)` | Environment variables, from the given `KEY=VALUE` list. |
| `Flags[T]()` | Command line flags, from `os.Args[1:]`. |
| `FlagSet[T](set, args)` | Command line flags, from your own flag set and args. |
| `File[T](dec, path)` | Config file at path, decoded with dec. |
| `Glob[T](dec, pattern)` | Every file matching the glob, in lexical order. |
| `FileFromFlag[T](dec, set, name, args)` | Config file the user gave with a flag, `-config app.json`. |

`Env`, `EnvFrom` and all four file parsers take options as last arguments, see
[extra keys](#extra-keys).

Nothing is magic about the order. Put the sources in the order you want them to win, and put
your own parsers among them wherever they belong.

## Names

The name of a field is derived from the Go field name, `ListenAddr` becomes `listen-addr`.
Nested structs are joined with a dash, env vars use the same name in upper case with underscores.

```go
type Config struct {
    ListenAddr string // -listen-addr, LISTEN_ADDR
    Server struct {
        TLSCert string // -server-tls-cert, SERVER_TLS_CERT
    }
}
```

In config files both the generated name and the Go field name are accepted, and matching
ignores case, dashes and underscores. `{"server": {"tls-cert": "a.pem"}}` and
`{"Server": {"TLSCert": "a.pem"}}` do the same thing.

## Tags

| Tag | Purpose |
| --- | --- |
| `cnfg:"name"` | Use the given name instead of the generated one. |
| `cnfg:"-"` | Leave the field out of all sources. |
| `usage:"text"` | Document the field in the usage output. |
| `json:"name"` | Used as the name when there is no `cnfg` tag. |

## Config files

`File` takes the function that decodes the bytes into a `*map[string]any`, which is what
`encoding/json`, `go.yaml.in/yaml/v3` and `github.com/BurntSushi/toml` all give you, so the
format is simply the library you already import:

```go
import "go.yaml.in/yaml/v3"

cfg, err := cnfg.Parse(defaults, cnfg.File[Config](yaml.Unmarshal, "/etc/app/config.yaml"))
```

A config file that is not there is a no op, since that is the one source allowed to be
missing, and your defaults are the config in that case. A file that is there but cannot be
read or decoded is still an error, wrapping `ErrReadFile` or `ErrDecodeFile`.

Anything with that signature works, so a format cnfg has never heard of needs no support from
cnfg:

```go
cfg, err := cnfg.Parse(defaults, cnfg.File[Config](hcl.Unmarshal, "/etc/app/config.hcl"))
```

Values from files go through the same parsing as env vars and flags, so a duration is written
as `"30s"` and a `net.IP` as `"10.0.0.1"` in every source.

To let the user point at a config file with a flag, give `FileFromFlag` and `FlagSet` the same
flag set and args. `FileFromFlag` registers the flag and reads the file before the other sources,
so the file is loaded even though it was named on the command line:

```go
set := flag.NewFlagSet("app", flag.ContinueOnError)
args := os.Args[1:]

cfg, err := cnfg.Parse(defaults,
    cnfg.FileFromFlag[Config](json.Unmarshal, set, "config", args),
    cnfg.Env[Config]("APP"),
    cnfg.FlagSet[Config](set, args),
)
```

## Drop-in directories

`Glob` reads a whole `conf.d` directory the way the rest of `/etc` does: every file the
pattern matches, in lexical order, each one on top of the last. A directory that is not there
is a no op like a missing file, so the usual base file plus drop-ins looks like this:

```go
import "go.yaml.in/yaml/v3"

cfg, err := cnfg.Parse(defaults,
    cnfg.File[Config](yaml.Unmarshal, "/etc/app/config.yaml"),
    cnfg.Glob[Config](yaml.Unmarshal, "/etc/app/config.d/*.conf"),
    cnfg.Env[Config]("APP"),
    cnfg.Flags[Config](),
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
several programs read and unfriendly to a typo. Pass `cnfg.Strict` to a source and it fails on
anything it cannot place:

```go
cfg, err := cnfg.Parse(defaults,
    cnfg.File[Config](yaml.Unmarshal, "/etc/app/config.yaml", cnfg.Strict),
    cnfg.Env[Config]("APP", cnfg.Strict),
    cnfg.Flags[Config](),
)
```

```
/etc/app/config.yaml: no field for server.tls-cret
APP: no field for APP_ADRR
```

The error wraps `ErrUnknownField` and names the first key it could not place, nested ones
joined with a dot and slice elements with their index. Keys under a `map` or an `any` field
are values rather than names, so they are left alone. Flags are strict on their own, an unknown flag has
always been an error.

Failing is one thing you can do with those keys. `cnfg.OnUnknown` hands them to you instead,
one call per key, and the error your function returns is what ends the parse, so returning nil
keeps it going:

```go
extras := cnfg.OnUnknown(func(u cnfg.Unknown) error {
    log.Printf("%s: ignoring %s=%v", u.Source, u.Key, u.Value)
    return nil
})

cfg, err := cnfg.Parse(defaults,
    cnfg.Glob[Config](yaml.Unmarshal, "/etc/app/config.d/*.conf", extras),
    cnfg.Env[Config]("APP", extras),
)
```

`Source` is the file the key came from, or the env prefix it was looked up with, so a drop-in
directory reports the file that carries the typo. `Value` is what the file or the environment
had under that key, and the keys of one source arrive in order.

Two handlers come ready made, both of them `OnUnknown` with a function in it:

| Option | Does |
| --- | --- |
| `cnfg.Strict` | Ends the parse on the first key, with the error above. |
| `cnfg.LogUnknown` | Warns about every key with the default `slog` logger and goes on. |

```
level=WARN msg="config key with no field" source=/etc/app/config.yaml key=worker_cont
```

`LogUnknown` leaves the value out of the log on purpose, a mistyped key can still carry a
secret.

For env vars the prefix is what tells yours from the rest of the environment, so `APP_ADRR` is
reported while `PATH` is not, and an `Env` that reports extras without a prefix fails with
`ErrNoPrefix` rather than reading the whole environment as yours.

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

Rules beyond the type, a range or a field that has to be set, are yours. They are a parser
like any other, and a plain function is enough for most configs:

```go
func validate(cfg Config) (Config, error) {
    if cfg.Workers < 1 {
        return cfg, fmt.Errorf("workers must be at least 1, got %d", cfg.Workers)
    }
    return cfg, nil
}

cfg, err := cnfg.Parse(defaults, cnfg.Env[Config]("APP"), cnfg.Flags[Config](), validate)
```

Rules that live in struct tags come from [github.com/go-cnfg/validator](https://github.com/go-cnfg/validator),
which wraps [go-playground/validator](https://github.com/go-playground/validator) as a parser:

```go
import "github.com/go-cnfg/validator"

type Config struct {
    Addr    string        `validate:"required,hostname_port"`
    Workers int           `validate:"gte=1,lte=100"`
    Level   string        `validate:"oneof=debug info warn"`
    Timeout time.Duration `validate:"min=1s"`
}

cfg, err := cnfg.Parse(defaults,
    cnfg.Env[Config]("APP"),
    cnfg.Flags[Config](),
    validator.Validate[Config](),
)
```

`validator.With[Config](v)` takes a validator you set up yourself, for your own rules or a
different tag name. Put the check last so every source has been read, and remember that the
parse stops there: `Parse` returns the zero value of your config together with the error.

## Flags

Flags are registered in a normal `flag.FlagSet`, so `-h` prints the usual listing with the types
and defaults from your struct:

```
$ app -h
Usage of app:
  -config string
    	path to config file
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
cfg, err := cnfg.Parse(defaults, cnfg.Flags[Config]())
if errors.Is(err, flag.ErrHelp) {
    return
}
```

Use `FlagSet` when you want to name the flag set, write the usage somewhere else, register flags
of your own or read the positional args that were left over:

```go
set := flag.NewFlagSet("app", flag.ContinueOnError)

cfg, err := cnfg.Parse(defaults, cnfg.FlagSet[Config](set, os.Args[1:]))
if err != nil {
    log.Fatal(err)
}

log.Println(set.Args())
```
