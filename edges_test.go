package cnfg_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-cnfg/cnfg"
)

type Nested struct {
	Max_conns int
	hidden    string //nolint:unused // only here to be skipped by the parsers
}

type Edges struct {
	Addr    string
	Nums    []int
	When    *time.Time
	Ptr     *int
	Bytes   []byte
	Level   Verbosity
	Any     any
	Limits  *Nested
	Counts  map[string]int
	Weird   map[complex128]string
	Meta    map[int]string
	Ignored chan int
}

func edgeFile(t *testing.T, content string) cnfg.Parser[Edges] {
	t.Helper()

	return cnfg.File[Edges](json.Unmarshal, writeFile(t, "edges.json", content))
}

func TestNilPointerStructIsFilled(t *testing.T) {
	cfg, err := cnfg.Parse(Edges{}, flags[Edges]("-limits-max-conns", "7"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Limits == nil || cfg.Limits.Max_conns != 7 {
		t.Errorf("nil pointer struct: got %+v", cfg.Limits)
	}
}

func TestPointerTextUnmarshaler(t *testing.T) {
	cfg, err := cnfg.Parse(Edges{}, flags[Edges]("-when", "2026-09-15T00:00:00Z"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.When == nil || cfg.When.Year() != 2026 {
		t.Errorf("pointer time: got %v", cfg.When)
	}
}

func TestFileValueShapes(t *testing.T) {
	cfg, err := cnfg.Parse(Edges{Addr: ":8080"}, edgeFile(t, `{
		"addr": 8080,
		"nums": null,
		"ptr": "42",
		"any": {"free": "form"},
		"counts": {"a": 1}
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "number into string", "8080", cfg.Addr)
	assertEqual(t, "null leaves the default", 0, len(cfg.Nums))
	if cfg.Ptr == nil || *cfg.Ptr != 42 {
		t.Errorf("string into pointer: got %v", cfg.Ptr)
	}
	if tree, ok := cfg.Any.(map[string]any); !ok || tree["free"] != "form" {
		t.Errorf("interface field: got %#v", cfg.Any)
	}
	assertEqual(t, "map value", 1, cfg.Counts["a"])
}

func TestFileValueErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    error
	}{
		{name: "bool into int", content: `{"ptr": true}`, want: cnfg.ErrDecodeFile},
		{name: "map key", content: `{"meta": {"x": "a"}}`, want: cnfg.ErrDecodeFile},
		{name: "map value", content: `{"counts": {"a": "x"}}`, want: cnfg.ErrDecodeFile},
		{name: "map key type", content: `{"weird": {"1": "x"}}`, want: cnfg.ErrUnsupportedType},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := cnfg.Parse(Edges{}, edgeFile(t, tc.content)); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSliceElementError(t *testing.T) {
	_, err := cnfg.Parse(Edges{}, env[Edges]("NUMS=1,x"))
	if !errors.Is(err, cnfg.ErrInvalidValue) {
		t.Fatalf("got %v, want ErrInvalidValue", err)
	}
}

func TestUnexportedAndUnderscoreNames(t *testing.T) {
	buf := &bytes.Buffer{}
	set := flag.NewFlagSet("app", flag.ContinueOnError)
	set.SetOutput(buf)
	set.Bool("quiet", false, "keep it down")

	when := time.Unix(0, 0).UTC()
	ptr := 3
	_, _ = cnfg.Parse(Edges{When: &when, Ptr: &ptr, Bytes: []byte("hi")}, cnfg.FlagSet[Edges](set, []string{"-h"}))

	for _, want := range []string{
		"-limits-max-conns int",
		"-bytes string\n    \t(default hi)",
		"-level value",
		"-ptr int\n    \t(default 3)",
		"-quiet\n    \tkeep it down",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("usage missing %q:\n%s", want, buf.String())
		}
	}
	if strings.Contains(buf.String(), "hidden") {
		t.Errorf("unexported field should be skipped:\n%s", buf.String())
	}
}

func TestFlagsFromArgs(t *testing.T) {
	args := os.Args
	t.Cleanup(func() { os.Args = args })
	os.Args = []string{"app", "-addr", ":4444"}

	cfg, err := cnfg.Parse(Edges{}, cnfg.Flags[Edges]())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertEqual(t, "addr from os.Args", ":4444", cfg.Addr)
}

func TestBrokenFileIsStillAnError(t *testing.T) {
	broken := writeFile(t, "broken.json", "{")

	if _, err := cnfg.Parse(Edges{}, cnfg.File[Edges](json.Unmarshal, broken)); !errors.Is(err, cnfg.ErrDecodeFile) {
		t.Errorf("got %v, want ErrDecodeFile", err)
	}
}

func TestFileFromFlagNotStruct(t *testing.T) {
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.SetOutput(io.Discard)

	_, err := cnfg.Parse(42, cnfg.FileFromFlag[int](json.Unmarshal, set, "config", nil))
	if !errors.Is(err, cnfg.ErrNotStruct) {
		t.Errorf("got %v, want ErrNotStruct", err)
	}
}
