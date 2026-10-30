package dubi

import (
	"math"
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestMappingJSONIntegerNumbers(t *testing.T) {
	testRuntime()
	t.Cleanup(Reset)
	decoded, err := Decode("json", `{"price":1000000,"unsigned":1e6,"prices":[1000000,1e6,1000000.0],"item":{"price":1000000}}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	mapped, result := Mapping(Vars{
		"price":    {Type: "int", Required: true},
		"unsigned": {Type: "uint64"},
		"prices":   {Type: "[int64]"},
		"item":     {Children: Vars{"price": {Type: "int64"}}},
	}, decoded.(Map))
	if result != nil && result.Fail() {
		t.Fatal(result)
	}
	if mapped["price"] != int64(1000000) || mapped["unsigned"] != uint64(1000000) || mapped["item"].(Map)["price"] != int64(1000000) {
		t.Fatalf("mapped values: %#v", mapped)
	}
	for _, value := range mapped["prices"].([]any) {
		if value != int64(1000000) {
			t.Fatalf("mapped price: %#v", value)
		}
	}
}

func TestIntegerConversionPreservesValuesAndBounds(t *testing.T) {
	type namedFloat float64
	b := &basicModule{}
	for _, test := range []struct {
		name, kind  string
		input, want any
	}{
		{"float64", "int64", float64(1000000), int64(1000000)},
		{"float32", "int64", float32(1000000), int64(1000000)},
		{"named float", "int64", namedFloat(1000000), int64(1000000)},
		{"negative", "int64", float64(-1000000), int64(-1000000)},
		{"integer string", "int64", "1000000", int64(1000000)},
		{"max signed integer", "int64", int64(math.MaxInt64), int64(math.MaxInt64)},
		{"max unsigned integer", "uint64", uint64(math.MaxUint64), uint64(math.MaxUint64)},
		{"exact large float", "int64", math.Nextafter(0x1p63, 0), int64(9223372036854774784)},
		{"exact large unsigned float", "uint64", math.Nextafter(0x1p64, 0), uint64(18446744073709549568)},
		{"signed minimum", "int64", float64(-0x1p63), int64(math.MinInt64)},
		{"small signed boundary", "int8", float64(127), int64(127)},
		{"small unsigned boundary", "uint8", float64(255), uint64(255)},
		{"negative zero", "uint64", math.Copysign(0, -1), uint64(0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := b.convert(test.kind, test.input, Var{})
			if err != nil || got != test.want {
				t.Fatalf("convert(%s, %v) = %#v, %v; want %#v", test.kind, test.input, got, err, test.want)
			}
		})
	}
}

func TestIntegerConversionRejectsFractionalAndOutOfRangeValues(t *testing.T) {
	b := &basicModule{}
	for _, test := range []struct {
		name, kind string
		input      any
	}{
		{"fraction", "int64", 1000000.5},
		{"unsigned fraction", "uint64", 1000000.5},
		{"small fraction", "int64", 0.000001},
		{"signed overflow", "int64", float64(0x1p63)},
		{"unsigned overflow", "uint64", float64(0x1p64)},
		{"int8 overflow", "int8", float64(128)},
		{"uint8 overflow", "uint8", float64(256)},
		{"negative unsigned", "uint64", float64(-1)},
		{"NaN", "int64", math.NaN()},
		{"infinity", "int64", math.Inf(1)},
		{"fraction string", "int64", "1.5"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got, err := b.convert(test.kind, test.input, Var{}); err == nil {
				t.Fatalf("convert(%s, %v) accepted invalid integer: %#v", test.kind, test.input, got)
			}
		})
	}
}
