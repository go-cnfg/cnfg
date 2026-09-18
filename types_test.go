package cnfg_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-cnfg/cnfg"
)

type Verbosity struct {
	name string
}

func (l *Verbosity) String() string { return l.name }

func (l *Verbosity) Set(s string) error {
	if s != "debug" && s != "info" {
		return errors.New("unknown level")
	}
	l.name = s
	return nil
}

type Item struct {
	Name string
	Port int
}

type Types struct {
	Int8      int8
	Uint      uint
	F32       float32
	Ptr       *int
	Bytes     []byte
	Nums      []int
	When      time.Time
	Verbosity Verbosity
	Items     []Item
	Ignore    chan int
}

func TestTypesFromFlags(t *testing.T) {
	cfg, err := cnfg.Parse(Types{}, flags(
		"-int8", "-8",
		"-uint", "0x10",
		"-f32", "1.25",
		"-ptr", "42",
		"-bytes", "raw",
		"-nums", "1,2,3",
		"-when", "2024-01-02T03:04:05Z",
		"-verbosity", "debug",
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "int8", int8(-8), cfg.Int8)
	assertEqual(t, "uint with base prefix", uint(16), cfg.Uint)
	assertEqual(t, "float32", float32(1.25), cfg.F32)
	assertEqual(t, "pointer", 42, *cfg.Ptr)
	assertEqual(t, "byte slice", "raw", string(cfg.Bytes))
	assertEqual(t, "time", "2024-01-02T03:04:05Z", cfg.When.Format(time.RFC3339))
	assertEqual(t, "flag.Value", "debug", cfg.Verbosity.String())
	if !reflect.DeepEqual([]int{1, 2, 3}, cfg.Nums) {
		t.Errorf("int slice: got %v", cfg.Nums)
	}
}

func TestTypesFromFile(t *testing.T) {
	file := writeFile(t, "types.json", `{
		"int8": 8,
		"ptr": 7,
		"bytes": "raw",
		"nums": [1, 2],
		"when": "2024-01-02T03:04:05Z",
		"verbosity": "info",
		"items": [{"name": "a", "port": 1}, {"name": "b", "port": 2}],
		"unknown": true
	}`)

	cfg, err := cnfg.Parse(Types{}, cnfg.File(json.Unmarshal, file))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertEqual(t, "int8", int8(8), cfg.Int8)
	assertEqual(t, "pointer", 7, *cfg.Ptr)
	assertEqual(t, "byte slice", "raw", string(cfg.Bytes))
	assertEqual(t, "time", "2024-01-02T03:04:05Z", cfg.When.Format(time.RFC3339))
	assertEqual(t, "flag.Value", "info", cfg.Verbosity.String())
	if want := []Item{{Name: "a", Port: 1}, {Name: "b", Port: 2}}; !reflect.DeepEqual(want, cfg.Items) {
		t.Errorf("slice of structs: got %v", cfg.Items)
	}
	if !reflect.DeepEqual([]int{1, 2}, cfg.Nums) {
		t.Errorf("int slice: got %v", cfg.Nums)
	}
}

func TestInvalidFileValues(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "wrong scalar", content: `{"nums": ["a"]}`, want: "nums"},
		{name: "list as struct", content: `{"items": {"a": 1}}`, want: "items"},
		{name: "struct as scalar", content: `{"items": [1]}`, want: "items"},
		{name: "flag value", content: `{"verbosity": "nope"}`, want: "unknown level"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file := writeFile(t, "types.json", tc.content)
			_, err := cnfg.Parse(Types{}, cnfg.File(json.Unmarshal, file))
			if !errors.Is(err, cnfg.ErrDecodeFile) {
				t.Fatalf("got %v, want ErrDecodeFile", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should mention %q", err, tc.want)
			}
		})
	}
}

func TestEnvSlices(t *testing.T) {
	cfg, err := cnfg.Parse(Types{Nums: []int{9}}, env("NUMS="))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Nums) != 0 {
		t.Errorf("empty env should clear the slice, got %v", cfg.Nums)
	}
}
