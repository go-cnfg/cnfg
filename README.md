[![Go Reference](https://pkg.go.dev/badge/github.com/go-cnfg/cnfg.svg)](https://pkg.go.dev/github.com/go-cnfg/cnfg) ![main](https://github.com/go-cnfg/cnfg/actions/workflows/go.yaml/badge.svg?branch=main)

# cnfg

Dead simple zero dependency config parser.

You declare your config as a plain struct, fill it with the defaults you want and hand it to
`cnfg.Parse` together with the sources you want to read. Sources are applied in the order you
give them, so the last one wins.

```go
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
        cnfg.Optional(cnfg.File[Config]("/etc/app/config.json")),
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

- [Parsers](#parsers)
- [Names](#names)
- [Tags](#tags)
- [Config files](#config-files)
- [Types](#types)
- [Validation](#validation)
- [Flags](#flags)

## Parsers

A parser is anything that takes a config and gives back a config:

```go
type Parser[T any] func(T) (T, error)
```

`File`, `Env` and `Flags` are just parsers that happen to read a source, and your own
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
| `File[T](path)` | Config file at path, decoder picked by extension. |
| `Optional(p)` | Wraps a parser so that a missing file is not an error. |
| `FileFlag[T](set, name, args)` | Config file the user gave with a flag, `-config app.json`. |
| `Env[T](prefix)` | Environment variables, from `os.Environ()`. |
| `EnvFrom[T](prefix, environ)` | Environment variables, from the given `KEY=VALUE` list. |
| `Flags[T]()` | Command line flags, from `os.Args[1:]`. |
| `FlagSet[T](set, args)` | Command line flags, from your own flag set and args. |

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

JSON is understood out of the box and any other format can be plugged in by adding a decoder.
A decoder is any function that decodes bytes into a `*map[string]any`, which `encoding/json`,
`gopkg.in/yaml.v3` and `github.com/BurntSushi/toml` all are.

```go
cnfg.Decoders[".yaml"] = yaml.Unmarshal

cfg, err := cnfg.Parse(defaults, cnfg.Optional(cnfg.File[Config]("/etc/app/config.yaml")))
```

Values from files go through the same parsing as env vars and flags, so a duration is written
as `"30s"` and a `net.IP` as `"10.0.0.1"` in every source.

To let the user point at a config file with a flag, give `FileFlag` and `FlagSet` the same
flag set and args. `FileFlag` registers the flag and reads the file before the other sources,
so the file is loaded even though it was named on the command line:

```go
set := flag.NewFlagSet("app", flag.ContinueOnError)
args := os.Args[1:]

cfg, err := cnfg.Parse(defaults,
    cnfg.FileFlag[Config](set, "config", args),
    cnfg.Env[Config]("APP"),
    cnfg.FlagSet[Config](set, args),
)
```

## Types

Anything that can be built from a single string works: strings, bools, ints, uints, floats,
`time.Duration`, `[]byte` and every type implementing `encoding.TextUnmarshaler` or `flag.Value`,
like `net.IP` and `time.Time`. Slices of those are comma separated on the command line and in
env vars, `-hosts a,b,c`.

Maps and slices of structs can only be filled from a config file since there is no sane way to
express them as a flag. They are simply skipped by the flag and env sources.

## Validation

cnfg does not validate anything, it only fills the struct, so validation is a parser like any
other. A plain function is enough for most configs:

```go
func validate(cfg Config) (Config, error) {
    if cfg.Workers < 1 {
        return cfg, fmt.Errorf("workers must be at least 1, got %d", cfg.Workers)
    }
    return cfg, nil
}

cfg, err := cnfg.Parse(defaults, cnfg.Env[Config]("APP"), cnfg.Flags[Config](), validate)
```

A validator that reads struct tags, like [go-playground/validator](https://github.com/go-playground/validator),
bolts on the same way and keeps the rules next to the fields:

```go
type Config struct {
    Addr    string        `validate:"required,hostname_port"`
    Workers int           `validate:"gte=1,lte=100"`
    Level   string        `validate:"oneof=debug info warn"`
    Timeout time.Duration `validate:"min=1s"`
}

var v = validator.New()

func validate(cfg Config) (Config, error) { return cfg, v.Struct(cfg) }

cfg, err := cnfg.Parse(defaults, cnfg.Env[Config]("APP"), cnfg.Flags[Config](), validate)
```

Put it last so every source has been read, and remember that the parse stops there: `Parse`
returns the zero value of your config together with the error.

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
