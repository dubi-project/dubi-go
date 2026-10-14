package dubi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	base "github.com/dubi-project/dubi-go/base"
)

// Codec is a named dynamic encoder and decoder. It is registered through the
// same Registry as every other Dubi component and kept per Runtime.
type Codec struct {
	Name   string
	Alias  []string
	Encode func(base.Any) (base.Any, error)
	Decode func(base.Any, base.Any) (base.Any, error)
}

func (Codec) Scope() string { return "codec" }

type codecModule struct{}

type codecInstance struct {
	codecs map[string]Codec
}

func init() { Mount(&codecModule{}) }

func (*codecModule) New() Instance {
	instance := &codecInstance{codecs: make(map[string]Codec)}
	for name, codec := range builtinCodecs() {
		instance.codecs[name] = codec
	}
	return instance
}

func (c *codecInstance) Register(name string, value any) {
	switch item := value.(type) {
	case Codec:
		c.register(name, item)
	case *Codec:
		if item != nil {
			c.register(name, *item)
		}
	}
}

func (c *codecInstance) register(name string, item Codec) {
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

func (*codecInstance) Config(base.Map) error { return nil }
func (*codecInstance) Setup(*Context) error  { return nil }
func (*codecInstance) Open(*Context) error   { return nil }
func (*codecInstance) Start(*Context) error  { return nil }
func (*codecInstance) Stop(*Context) error   { return nil }
func (*codecInstance) Close(*Context) error  { return nil }

func (rt *Runtime) Encode(name string, value base.Any) (base.Any, error) {
	return rt.codecInstance().encode(name, value)
}

func (rt *Runtime) Decode(name string, data base.Any, target base.Any) (base.Any, error) {
	return rt.codecInstance().decode(name, data, target)
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

func (c *codecInstance) encode(name string, value base.Any) (base.Any, error) {
	codec, ok := c.codecs[strings.ToLower(strings.TrimSpace(name))]
	if !ok || codec.Encode == nil {
		return nil, fmt.Errorf("dubi: codec %q is not registered", name)
	}
	return codec.Encode(value)
}

func (c *codecInstance) decode(name string, data base.Any, target base.Any) (base.Any, error) {
	codec, ok := c.codecs[strings.ToLower(strings.TrimSpace(name))]
	if !ok || codec.Decode == nil {
		return nil, fmt.Errorf("dubi: codec %q is not registered", name)
	}
	return codec.Decode(data, target)
}

func (rt *Runtime) codecInstance() *codecInstance {
	if rt != nil && rt.codec != nil {
		return rt.codec
	}
	return codecForRuntime(nil)
}

var standaloneCodec = func() *codecInstance {
	return (&codecModule{}).New().(*codecInstance)
}()

func codecForRuntime(rt *Runtime) *codecInstance {
	if rt != nil && rt.codec != nil {
		return rt.codec
	}
	return standaloneCodec
}

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
