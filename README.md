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
        cnfg.Optional(json.File[Config]("/etc/app/config.json")),
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

- [Modules](#modules)
- [Parsers](#parsers)
- [Names](#names)
- [Tags](#tags)
- [Config files](#config-files)
- [Drop-in directories](#drop-in-directories)
- [Types](#types)
- [Validation](#validation)
- [Flags](#flags)

## Modules

The core module reads env vars and flags and needs nothing outside the standard library.
Config file formats and validation live in modules of their own, so a dependency is only
pulled in when you ask for it:

| Module | Provides | Depends on |
| --- | --- | --- |
| `github.com/go-cnfg/cnfg` | `Parse`, `Env`, `Flags`, `Decode` | standard library |
| `github.com/go-cnfg/cnfg/json` | `json.File`, `json.FileFlag` | standard library |
| `github.com/go-cnfg/cnfg/yaml` | `yaml.File`, `yaml.FileFlag` | `go.yaml.in/yaml/v3` |
| `github.com/go-cnfg/cnfg/toml` | `toml.File`, `toml.FileFlag` | `github.com/BurntSushi/toml` |
| `github.com/go-cnfg/cnfg/validator` | `validator.Validate` | `github.com/go-playground/validator/v10` |

```sh
go get github.com/go-cnfg/cnfg
go get github.com/go-cnfg/cnfg/yaml
```

## Parsers

A parser is anything that takes a config and gives back a config:

```go
type Parser[T any] func(T) (T, error)
```

`Env`, `Flags` and the file parsers are just parsers that happen to read a source, and your own
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
| `Decode[T](dec, path)` | Config file at path, decoded with dec. |
| `DecodeGlob[T](dec, pattern)` | Every file matching the glob, in lexical order. |
| `DecodeDir[T](dec, dir)` | Every `.conf` file of a drop-in directory. |
| `DecodeFlag[T](dec, set, name, args)` | Config file the user gave with a flag, `-config app.json`. |
| `Optional(p)` | Wraps a parser so that a missing file is not an error. |
| `json.File[T](path)`, `yaml.File[T](path)`, `toml.File[T](path)` | Config file in that format. |
| `json.FileFlag[T](set, name, args)` and friends | Config file the user gave with a flag. |
| `validator.Validate[T]()` | Nothing, it checks the config that the other parsers filled. |

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

Every format is a package of its own, so you import the one you want and get a `File` parser
for it:

```go
import "github.com/go-cnfg/cnfg/yaml"

cfg, err := cnfg.Parse(defaults, cnfg.Optional(yaml.File[Config]("/etc/app/config.yaml")))
```

`Optional` turns a missing file into a no op, without it a missing file is an error wrapping
`fs.ErrNotExist`. Any other format works the same way with `cnfg.Decode`, which takes the
function that decodes the bytes into a `*map[string]any`:

```go
cfg, err := cnfg.Parse(defaults, cnfg.Decode[Config](hcl.Unmarshal, "/etc/app/config.hcl"))
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
    json.FileFlag[Config](set, "config", args),
    cnfg.Env[Config]("APP"),
    cnfg.FlagSet[Config](set, args),
)
```

## Drop-in directories

`DecodeDir` reads a whole `conf.d` directory the way the rest of `/etc` does: every `.conf`
file in it, in lexical order, each one on top of the last. A directory that is not there is a
no op, so the usual base file plus drop-ins looks like this:

```go
import "go.yaml.in/yaml/v3"

cfg, err := cnfg.Parse(defaults,
    cnfg.Optional(cnfg.Decode[Config](yaml.Unmarshal, "/etc/app/config.yaml")),
    cnfg.DecodeDir[Config](yaml.Unmarshal, "/etc/app/config.d"),
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
wins. `DecodeDir` is `DecodeGlob` with the usual `*.conf` pattern, so reach for `DecodeGlob`
when the files are named something else, `cnfg.DecodeGlob[Config](yaml.Unmarshal, "/etc/app/config.d/*.yaml")`.

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

Rules that live in struct tags come from the `validator` module, which wraps
[go-playground/validator](https://github.com/go-playground/validator):

```go
import "github.com/go-cnfg/cnfg/validator"

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
