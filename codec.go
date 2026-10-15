package dubi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	base "github.com/dubi-project/dubi-go/base"
)

// Codec is a named dynamic encoder and decoder. It is registered through the
// same Registry as every other Dubi component and kept by the process module.
type Codec struct {
	Name   string
	Alias  []string
	Encode func(base.Any) (base.Any, error)
	Decode func(base.Any, base.Any) (base.Any, error)
}

func (Codec) Scope() string { return "codec" }

type codecModule struct {
	codecs map[string]Codec
}

var codec = &codecModule{codecs: make(map[string]Codec)}

func init() {
	for name, item := range builtinCodecs() {
		codec.codecs[name] = item
	}
	Mount(codec)
}

func (c *codecModule) Register(name string, value any) {
	switch item := value.(type) {
	case Codec:
		c.register(name, item)
	case *Codec:
		if item != nil {
			c.register(name, *item)
		}
	}
}

func (c *codecModule) Reset() {
	c.codecs = builtinCodecs()
}

func (c *codecModule) register(name string, item Codec) {
	if item.Name == "" {
		item.Name = name
	}
	keys := append([]string{name}, item.Alias...)
	for _, key := range keys {
		key = strings.ToLower(strings.TrimSpace(key))
		if key != "" {
			c.codecs[key] = item
		}
	}
}

func (*codecModule) Config(base.Map) error { return nil }
func (*codecModule) Setup(*Context) error  { return nil }
func (*codecModule) Open(*Context) error   { return nil }
func (*codecModule) Start(*Context) error  { return nil }
func (*codecModule) Stop(*Context) error   { return nil }
func (*codecModule) Close(*Context) error  { return nil }

func (rt *Runtime) Encode(name string, value base.Any) (base.Any, error) {
	return rt.codecModule().encode(name, value)
}

func (rt *Runtime) Decode(name string, data base.Any, target base.Any) (base.Any, error) {
	return rt.codecModule().decode(name, data, target)
}

func (rt *Runtime) Marshal(name string, value base.Any) ([]byte, error) {
	encoded, err := rt.Encode(name, value)
	if err != nil {
		return nil, err
	}
	if result, ok := encoded.([]byte); ok {
		return result, nil
	}
	return nil, fmt.Errorf("dubi: codec %q returned %T, want []byte", name, encoded)
}

func (rt *Runtime) Unmarshal(name string, data []byte, target base.Any) error {
	_, err := rt.Decode(name, data, target)
	return err
}

func Encode(name string, value base.Any) (base.Any, error) {
	return DefaultRuntime().Encode(name, value)
}

func Decode(name string, data base.Any, target base.Any) (base.Any, error) {
	return DefaultRuntime().Decode(name, data, target)
}

func Marshal(name string, value base.Any) ([]byte, error) {
	return DefaultRuntime().Marshal(name, value)
}

func Unmarshal(name string, data []byte, target base.Any) error {
	return DefaultRuntime().Unmarshal(name, data, target)
}

func (c *codecModule) encode(name string, value base.Any) (base.Any, error) {
	codec, ok := c.codecs[strings.ToLower(strings.TrimSpace(name))]
	if !ok || codec.Encode == nil {
		return nil, fmt.Errorf("dubi: codec %q is not registered", name)
	}
	return codec.Encode(value)
}

func (c *codecModule) decode(name string, data base.Any, target base.Any) (base.Any, error) {
	codec, ok := c.codecs[strings.ToLower(strings.TrimSpace(name))]
	if !ok || codec.Decode == nil {
		return nil, fmt.Errorf("dubi: codec %q is not registered", name)
	}
	return codec.Decode(data, target)
}

func (rt *Runtime) codecModule() *codecModule {
	return codec
}

func codecForRuntime(*Runtime) *codecModule { return codec }

func builtinCodecs() map[string]Codec {
	jsonCodec := Codec{
		Encode: func(value base.Any) (base.Any, error) { return json.Marshal(value) },
		Decode: func(data base.Any, target base.Any) (base.Any, error) {
			bytes, err := codecBytes(data)
			if err != nil {
				return nil, err
			}
			if target != nil {
				return target, json.Unmarshal(bytes, target)
			}
			var value base.Any
			err = json.Unmarshal(bytes, &value)
			return normalizeBasicValue(value), err
		},
	}
	return map[string]Codec{
		"json": jsonCodec,
		"text": {
			Encode: func(value base.Any) (base.Any, error) { return []byte(fmt.Sprint(value)), nil },
			Decode: func(data base.Any, _ base.Any) (base.Any, error) {
				bytes, err := codecBytes(data)
				return string(bytes), err
			},
		},
		"base64": {
			Encode: func(value base.Any) (base.Any, error) {
				bytes, err := codecBytes(value)
				if err != nil {
					return nil, err
				}
				return base64.StdEncoding.EncodeToString(bytes), nil
			},
			Decode: func(data base.Any, _ base.Any) (base.Any, error) {
				return base64.StdEncoding.DecodeString(fmt.Sprint(data))
			},
		},
	}
}

func codecBytes(value base.Any) ([]byte, error) {
	switch item := value.(type) {
	case []byte:
		return item, nil
	case string:
		return []byte(item), nil
	default:
		return nil, fmt.Errorf("dubi: cannot decode %T as bytes", value)
	}
}
