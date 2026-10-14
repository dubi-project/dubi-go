package dubi

import (
	"context"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

// Meta carries request-wide state across Method, Service, and entry modules.
// It is intentionally dynamic and can be embedded in protocol-specific
// contexts.
type Meta struct {
	ctx context.Context

	id       string
	traceID  string
	spanID   string
	language string
	timezone *time.Location
	token    string
	result   Res
	runtime  *Runtime
}

// NewMeta creates a request metadata object with a background context and a
// unique request ID.
func NewMeta(runtimes ...*Runtime) *Meta {
	runtime := DefaultRuntime()
	if len(runtimes) > 0 && runtimes[0] != nil {
		runtime = runtimes[0]
	}
	return &Meta{
		ctx:      context.Background(),
		id:       newRequestID(),
		timezone: time.Local,
		runtime:  runtime,
	}
}

func (m *Meta) WithContext(ctx context.Context) *Meta {
	if ctx == nil {
		ctx = context.Background()
	}
	m.ctx = ctx
	return m
}

func (m *Meta) Context() context.Context {
	if m == nil || m.ctx == nil {
		return context.Background()
	}
	return m.ctx
}

// Id returns the immutable request ID.
func (m *Meta) Id() string {
	if m == nil {
		return ""
	}
	if m.id == "" {
		m.id = newRequestID()
	}
	return m.id
}

func (m *Meta) TraceId(value ...string) string {
	if len(value) > 0 {
		m.traceID = value[0]
	}
	return m.traceID
}

func (m *Meta) SpanId(value ...string) string {
	if len(value) > 0 {
		m.spanID = value[0]
	}
	return m.spanID
}

func (m *Meta) Language(value ...string) string {
	if len(value) > 0 {
		m.language = value[0]
	}
	return m.language
}

func (m *Meta) Timezone(value ...*time.Location) *time.Location {
	if len(value) > 0 && value[0] != nil {
		m.timezone = value[0]
	}
	if m.timezone == nil {
		return time.Local
	}
	return m.timezone
}

func (m *Meta) Token(value ...string) string {
	if len(value) > 0 {
		m.token = value[0]
	}
	return m.token
}

// String resolves a status or language key using this Meta's Runtime.
// Context-specific language selection is taken from Meta.Language().
func (m *Meta) String(key string, args ...Any) string {
	language := ""
	if m != nil {
		language = m.Language()
	}
	return m.runtimeOrDefault().String(language, key, args...)
}

// StatusCode returns the internal code for a status in this Meta's Runtime.
func (m *Meta) StatusCode(key string, defaults ...int) int {
	return m.runtimeOrDefault().StatusCode(key, defaults...)
}

// Mimetype resolves a MIME type using this Meta's Runtime.
func (m *Meta) Mimetype(ext string, defaults ...string) string {
	return m.runtimeOrDefault().Mimetype(ext, defaults...)
}

// Encode uses the codec registered on this Meta's Runtime.
func (m *Meta) Encode(name string, value Any) (Any, error) {
	return m.runtimeOrDefault().Encode(name, value)
}

// Decode uses the codec registered on this Meta's Runtime.
func (m *Meta) Decode(name string, data Any, target Any) (Any, error) {
	return m.runtimeOrDefault().Decode(name, data, target)
}

// Marshal encodes a value with the codec registered on this Meta's Runtime.
func (m *Meta) Marshal(name string, value Any) ([]byte, error) {
	return m.runtimeOrDefault().Marshal(name, value)
}

// Unmarshal decodes bytes with the codec registered on this Meta's Runtime.
func (m *Meta) Unmarshal(name string, data []byte, target Any) error {
	return m.runtimeOrDefault().Unmarshal(name, data, target)
}

// Mapping validates and converts values using this Meta's Runtime.
func (m *Meta) Mapping(config Vars, data Map, values ...Map) (Map, Res) {
	return m.runtimeOrDefault().Mapping(config, data, values...)
}

// PrepareMapping compiles a dynamic mapping definition for repeated calls on
// this Meta's Runtime.
func (m *Meta) PrepareMapping(config Vars) *MappingPlan {
	return m.runtimeOrDefault().PrepareMapping(config)
}

// ApplyMapping applies a mapping plan compiled for this Meta's Runtime.
func (m *Meta) ApplyMapping(plan *MappingPlan, data Map, values ...Map) (Map, Res) {
	return m.runtimeOrDefault().ApplyMapping(plan, data, values...)
}

func (m *Meta) runtimeOrDefault() *Runtime {
	if m != nil && m.runtime != nil {
		return m.runtime
	}
	return DefaultRuntime()
}

func (m *Meta) Result(value ...Res) Res {
	if len(value) > 0 {
		m.result = value[0]
	}
	return m.result
}

// Invoke calls a Method or Service through the Runtime attached to Meta.
func (m *Meta) Invoke(name string, values ...Map) Map {
	var value Map
	if len(values) > 0 {
		value = values[0]
	}
	data, result := invokeRuntime(m, name, value, false)
	m.result = result
	return data
}

// Execute calls a local Method only.
func (m *Meta) Execute(name string, values ...Map) Map {
	var value Map
	if len(values) > 0 {
		value = values[0]
	}
	data, result := invokeRuntime(m, name, value, true)
	m.result = result
	return data
}

func (m *Meta) InvokeOK(name string, values ...Map) bool {
	m.Invoke(name, values...)
	return m.result == nil || m.result.OK()
}

func (m *Meta) InvokeFail(name string, values ...Map) bool {
	return !m.InvokeOK(name, values...)
}
