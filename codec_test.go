package dubi

import (
	"context"
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestRuntimeCodecIsolatedAndBuiltInJSON(t *testing.T) {
	rt := New()
	rt.Register("upper", Codec{
		Encode: func(value Any) (Any, error) { return []byte(value.(string)), nil },
		Decode: func(value Any, _ Any) (Any, error) { return value, nil },
	})
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	data, err := rt.Marshal("json", Map{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	var decoded Map
	if err := rt.Unmarshal("json", data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["ok"] != true {
		t.Fatalf("decoded = %#v", decoded)
	}
	encoded, err := rt.Marshal("upper", "hello")
	if err != nil || string(encoded) != "hello" {
		t.Fatalf("custom codec = %q, %v", encoded, err)
	}
	meta := NewMeta(rt)
	encoded, err = meta.Marshal("upper", "from-meta")
	if err != nil || string(encoded) != "from-meta" {
		t.Fatalf("meta codec = %q, %v", encoded, err)
	}
}

func TestRuntimeCodecOverridesAreIsolated(t *testing.T) {
	first := New()
	first.Register("label", Codec{
		Encode: func(value Any) (Any, error) { return []byte("first:" + value.(string)), nil },
	})
	second := New()
	second.Register("label", Codec{
		Encode: func(value Any) (Any, error) { return []byte("second:" + value.(string)), nil },
	})
	if err := first.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := second.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer first.Close(context.Background())
	defer second.Close(context.Background())

	firstValue, err := NewMeta(first).Marshal("label", "value")
	if err != nil || string(firstValue) != "first:value" {
		t.Fatalf("first runtime codec = %q, %v", firstValue, err)
	}
	secondValue, err := NewMeta(second).Marshal("label", "value")
	if err != nil || string(secondValue) != "second:value" {
		t.Fatalf("second runtime codec = %q, %v", secondValue, err)
	}
}
