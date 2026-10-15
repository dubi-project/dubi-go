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

	id        string
	traceID   string
	spanID    string
	language  string
	timezone  *time.Location
	token     string
	verified  bool
	tokenData Token
	result    Res
}

// NewMeta creates a request metadata object with a background context and a
// unique request ID.
func NewMeta() *Meta {
	return (&Meta{}).Reset(context.Background())
}

// Reset prepares a Meta for a new request while preserving its allocation.
// Entry modules that pool their request context can use this to keep the
// metadata object beside the context instead of allocating it separately.
func (m *Meta) Reset(ctx context.Context) *Meta {
	if m == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.ctx = ctx
	m.id = newRequestID()
	m.traceID = ""
	m.spanID = ""
	m.language = ""
	m.timezone = time.Local
	m.token = ""
	m.verified = false
	m.tokenData = Token{}
	m.result = nil
	return m
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
		m.verified = false
		m.tokenData = Token{}
	}
	return m.token
}

// SignToken signs a Token using the process auth service and records it as the
// current verified Token when successful.
func (m *Meta) SignToken(value Token) (string, error) {
	encoded, issued, err := DefaultRuntime().issueToken(value)
	if err != nil {
		return "", err
	}
	m.token = encoded
	m.tokenData = issued
	m.verified = true
	return encoded, nil
}

// VerifyToken verifies and records a raw Token on this Meta.
func (m *Meta) VerifyToken(value string) (Token, error) {
	token, err := VerifyToken(value)
	m.token = value
	m.tokenData = Token{}
	m.verified = false
	if err != nil {
		return Token{}, err
	}
	m.tokenData = token
	m.verified = true
	return token, nil
}

// Auth issues an authenticated Token. Protocol-specific contexts can add
// their route Kind before calling SignToken; the base Meta leaves Kind empty.
func (m *Meta) Auth(payload ...Map) string {
	var value Map
	if len(payload) > 0 {
		value = payload[0]
	}
	token, err := m.SignToken(Token{Auth: true, Payload: value})
	if err != nil {
		m.result = ErrorResult(err)
		return ""
	}
	return token
}

// Signed reports whether the current Token passed signature, expiry, and
// revocation checks.
func (m *Meta) Signed() bool { return m != nil && m.verified }

// Authed reports whether the current Token is valid and authenticated.
func (m *Meta) Authed() bool { return m.Signed() && m.tokenData.Auth }

// Kind returns the current verified Token's identity kind.
func (m *Meta) Kind() string {
	if m == nil || !m.verified {
		return ""
	}
	return m.tokenData.Kind
}

// Payload returns a copy of the current verified Token payload.
func (m *Meta) Payload() Map {
	if m == nil || !m.verified {
		return nil
	}
	return m.tokenData.Payload.Clone()
}

// TokenData returns a copy of the current verified Token.
func (m *Meta) TokenData() Token {
	if m == nil || !m.verified {
		return Token{}
	}
	value := m.tokenData
	value.Payload = value.Payload.Clone()
	return value
}

// RevokeToken revokes a Token ID. The optional expiry bounds the local
// revocation record's lifetime.
func (m *Meta) RevokeToken(id string, expires ...int64) error {
	return RevokeToken(id, expires...)
}

// RevokeCurrentToken revokes the Token currently attached to this Meta.
func (m *Meta) RevokeCurrentToken() error {
	if m == nil || !m.verified || m.tokenData.ID == "" {
		return ErrTokenIDRequired
	}
	return RevokeToken(m.tokenData.ID, m.tokenData.Expires)
}

// String resolves a status or language key using the process language catalog.
// Context-specific language selection is taken from Meta.Language().
func (m *Meta) String(key string, args ...Any) string {
	language := ""
	if m != nil {
		language = m.Language()
	}
	return String(language, key, args...)
}

// StatusCode returns the process status code for a status key.
func (m *Meta) StatusCode(key string, defaults ...int) int {
	return StatusCode(key, defaults...)
}

// Mimetype resolves a MIME type using the process catalog.
func (m *Meta) Mimetype(ext string, defaults ...string) string {
	return Mimetype(ext, defaults...)
}

// Encode uses a codec registered in the process registry.
func (m *Meta) Encode(name string, value Any) (Any, error) {
	return Encode(name, value)
}

// Decode uses a codec registered in the process registry.
func (m *Meta) Decode(name string, data Any, target Any) (Any, error) {
	return Decode(name, data, target)
}

// Marshal encodes a value with a codec registered in the process registry.
func (m *Meta) Marshal(name string, value Any) ([]byte, error) {
	return Marshal(name, value)
}

// Unmarshal decodes bytes with a codec registered in the process registry.
func (m *Meta) Unmarshal(name string, data []byte, target Any) error {
	return Unmarshal(name, data, target)
}

// Mapping validates and converts values using the process basic catalog.
func (m *Meta) Mapping(config Vars, data Map, values ...Map) (Map, Res) {
	return Mapping(config, data, values...)
}

// PrepareMapping compiles a dynamic mapping definition for repeated calls on
// this Meta's process.
func (m *Meta) PrepareMapping(config Vars) *MappingPlan {
	return PrepareMapping(config)
}

// ApplyMapping applies a mapping plan compiled for this Meta's process.
func (m *Meta) ApplyMapping(plan *MappingPlan, data Map, values ...Map) (Map, Res) {
	return ApplyMapping(plan, data, values...)
}

func (m *Meta) Result(value ...Res) Res {
	if len(value) > 0 {
		m.result = value[0]
	}
	return m.result
}

// Invoke calls a Method or Service through the process registry.
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
