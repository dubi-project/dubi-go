package dubi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

const (
	tokenPrefix       = "dubi1"
	defaultTokenTTL   = 24 * time.Hour
	defaultCookieName = "dubi_token"
)

var (
	// ErrAuthUnavailable means the process has not completed Setup yet or does
	// not contain the built-in auth module.
	ErrAuthUnavailable = errors.New("dubi: auth is not available")
	// ErrTokenInvalid means the token is malformed or its signature is invalid.
	ErrTokenInvalid = errors.New("dubi: invalid token")
	// ErrTokenExpired means the token is outside its validity interval.
	ErrTokenExpired = errors.New("dubi: token expired")
	// ErrTokenRevoked means the token ID has been revoked.
	ErrTokenRevoked = errors.New("dubi: token revoked")
	// ErrTokenIDRequired means a revocation request did not contain an ID.
	ErrTokenIDRequired = errors.New("dubi: token ID is required")
)

// Token is the framework's signed authentication value. Payload stays
// dynamic so application identity data does not need a generic type or a
// second DTO layer.
type Token struct {
	ID      string
	Auth    bool
	Kind    string
	Payload Map
	Begin   int64
	Expires int64
}

type tokenEnvelope struct {
	ID      string `json:"id"`
	Auth    bool   `json:"auth"`
	Kind    string `json:"kind,omitempty"`
	Payload Map    `json:"payload,omitempty"`
	Begin   int64  `json:"begin"`
	Expires int64  `json:"expires"`
}

// authModule is a built-in process module. The implementation is local for
// now; a later Dubi-backed implementation can preserve the same process and
// Meta API.
type authModule struct {
	mu      sync.RWMutex
	secret  []byte
	cookie  string
	ttl     time.Duration
	revoked map[string]int64
}

var auth = &authModule{secret: newAuthSecret(), cookie: defaultCookieName, ttl: defaultTokenTTL, revoked: make(map[string]int64)}

func init() { Mount(auth) }

func (a *authModule) Config(global Map) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	section, ok := authConfigMap(global["auth"])
	if !ok {
		return nil
	}
	if value := strings.TrimSpace(fmt.Sprint(firstAuthValue(section, "secret", "key"))); value != "" && value != "<nil>" {
		a.secret = []byte(value)
	}
	if value := strings.TrimSpace(fmt.Sprint(firstAuthValue(section, "cookie", "cookie_name"))); value != "" && value != "<nil>" {
		a.cookie = value
	}
	if value, exists := firstAuthValueOK(section, "ttl", "expire", "expires"); exists {
		if ttl, ok := authDuration(value); ok && ttl > 0 {
			a.ttl = ttl
		}
	}
	return nil
}

func (*authModule) Register(string, any) {}

func (a *authModule) Reset() {
	a.mu.Lock()
	a.secret = newAuthSecret()
	a.cookie = defaultCookieName
	a.ttl = defaultTokenTTL
	a.revoked = make(map[string]int64)
	a.mu.Unlock()
}
func (*authModule) Setup(*Context) error { return nil }
func (*authModule) Open(*Context) error  { return nil }
func (*authModule) Start(*Context) error { return nil }
func (*authModule) Stop(*Context) error  { return nil }

func (a *authModule) Close(*Context) error {
	a.mu.Lock()
	clear(a.revoked)
	a.mu.Unlock()
	return nil
}

func (a *authModule) issue(value Token) (string, Token, error) {
	a.mu.RLock()
	secret := append([]byte(nil), a.secret...)
	ttl := a.ttl
	a.mu.RUnlock()
	if len(secret) == 0 {
		return "", Token{}, ErrAuthUnavailable
	}
	now := time.Now().Unix()
	if value.ID == "" {
		value.ID = NewTokenID()
	}
	if value.Begin == 0 {
		value.Begin = now
	}
	if value.Expires == 0 {
		value.Expires = now + int64(ttl/time.Second)
	}
	if value.Expires <= value.Begin {
		return "", Token{}, fmt.Errorf("dubi: token expiry must be after begin")
	}
	data, err := json.Marshal(tokenEnvelope{
		ID: value.ID, Auth: value.Auth, Kind: value.Kind,
		Payload: value.Payload, Begin: value.Begin, Expires: value.Expires,
	})
	if err != nil {
		return "", Token{}, fmt.Errorf("dubi: encode token: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	mac := tokenMAC(secret, encoded)
	result := tokenPrefix + "." + encoded + "." + base64.RawURLEncoding.EncodeToString(mac)
	return result, value, nil
}

func (a *authModule) verify(value string) (Token, error) {
	a.mu.RLock()
	secret := append([]byte(nil), a.secret...)
	a.mu.RUnlock()
	if len(secret) == 0 {
		return Token{}, ErrAuthUnavailable
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != tokenPrefix || parts[1] == "" || parts[2] == "" {
		return Token{}, ErrTokenInvalid
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(provided, tokenMAC(secret, parts[1])) {
		return Token{}, ErrTokenInvalid
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Token{}, ErrTokenInvalid
	}
	var envelope tokenEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.ID == "" || envelope.Begin <= 0 || envelope.Expires <= 0 {
		return Token{}, ErrTokenInvalid
	}
	now := time.Now().Unix()
	if now < envelope.Begin || now >= envelope.Expires {
		return Token{}, ErrTokenExpired
	}
	if a.isRevoked(envelope.ID, now) {
		return Token{}, ErrTokenRevoked
	}
	return Token{
		ID: envelope.ID, Auth: envelope.Auth, Kind: envelope.Kind,
		Payload: envelope.Payload, Begin: envelope.Begin, Expires: envelope.Expires,
	}, nil
}

func (a *authModule) revoke(id string, expires int64) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrTokenIDRequired
	}
	now := time.Now().Unix()
	if expires <= now {
		return nil
	}
	a.mu.Lock()
	if a.revoked == nil {
		a.revoked = make(map[string]int64)
	}
	a.cleanRevokedLocked(now)
	a.revoked[id] = expires
	a.mu.Unlock()
	return nil
}

func (a *authModule) isRevoked(id string, now int64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cleanRevokedLocked(now)
	expires, ok := a.revoked[id]
	return ok && expires > now
}

func (a *authModule) cleanRevokedLocked(now int64) {
	for id, expires := range a.revoked {
		if expires <= now {
			delete(a.revoked, id)
		}
	}
}

func tokenMAC(secret []byte, encoded string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(encoded))
	return mac.Sum(nil)
}

func newAuthSecret() []byte {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		panic("dubi: cannot generate auth secret: " + err.Error())
	}
	return secret
}

// NewTokenID creates a random identifier suitable for a Token ID or a
// revocation key.
func NewTokenID() string { return newUniqueID("tok") }

func (rt *Runtime) authModule() *authModule {
	return auth
}

func (rt *Runtime) issueToken(value Token) (string, Token, error) {
	auth := rt.authModule()
	if auth == nil {
		return "", Token{}, ErrAuthUnavailable
	}
	return auth.issue(value)
}

// SignToken signs a Token using the process auth service.
func (rt *Runtime) SignToken(value Token) (string, error) {
	result, _, err := rt.issueToken(value)
	return result, err
}

// VerifyToken verifies a Token and checks its local revocation table.
func (rt *Runtime) VerifyToken(value string) (Token, error) {
	auth := rt.authModule()
	if auth == nil {
		return Token{}, ErrAuthUnavailable
	}
	return auth.verify(strings.TrimSpace(value))
}

// RevokeToken revokes a Token ID until the supplied Unix expiry. When no
// expiry is supplied, the process default token lifetime is used.
func (rt *Runtime) RevokeToken(id string, expires ...int64) error {
	auth := rt.authModule()
	if auth == nil {
		return ErrAuthUnavailable
	}
	until := int64(0)
	if len(expires) > 0 {
		until = expires[0]
	} else {
		auth.mu.RLock()
		until = time.Now().Add(auth.ttl).Unix()
		auth.mu.RUnlock()
	}
	return auth.revoke(id, until)
}

// AuthCookieName returns the configured local auth cookie name.
func (rt *Runtime) AuthCookieName() string {
	auth := rt.authModule()
	if auth == nil {
		return defaultCookieName
	}
	auth.mu.RLock()
	defer auth.mu.RUnlock()
	if auth.cookie == "" {
		return defaultCookieName
	}
	return auth.cookie
}

// AuthCookieName returns the process auth cookie name.
func AuthCookieName() string { return DefaultRuntime().AuthCookieName() }

// AuthTTL returns the configured local Token lifetime.
func (rt *Runtime) AuthTTL() time.Duration {
	auth := rt.authModule()
	if auth == nil {
		return defaultTokenTTL
	}
	auth.mu.RLock()
	defer auth.mu.RUnlock()
	if auth.ttl <= 0 {
		return defaultTokenTTL
	}
	return auth.ttl
}

// AuthTTL returns the process auth token lifetime.
func AuthTTL() time.Duration { return DefaultRuntime().AuthTTL() }

// SignToken signs a Token using the process auth service.
func SignToken(value Token) (string, error) { return DefaultRuntime().SignToken(value) }

// VerifyToken verifies a Token using the process auth service.
func VerifyToken(value string) (Token, error) { return DefaultRuntime().VerifyToken(value) }

// RevokeToken revokes a Token ID on the process auth service.
func RevokeToken(id string, expires ...int64) error {
	return DefaultRuntime().RevokeToken(id, expires...)
}

func authConfigMap(value any) (Map, bool) {
	switch item := value.(type) {
	case Map:
		return item, true
	case map[string]any:
		result := Map{}
		for key, value := range item {
			result[key] = value
		}
		return result, true
	default:
		return nil, false
	}
}

func firstAuthValue(section Map, names ...string) any {
	value, _ := firstAuthValueOK(section, names...)
	return value
}

func firstAuthValueOK(section Map, names ...string) (any, bool) {
	for _, name := range names {
		for key, value := range section {
			if strings.EqualFold(strings.TrimSpace(key), name) {
				return value, true
			}
		}
	}
	return nil, false
}

func authDuration(value any) (time.Duration, bool) {
	switch item := value.(type) {
	case time.Duration:
		return item, true
	case int:
		return time.Duration(item) * time.Second, true
	case int64:
		return time.Duration(item) * time.Second, true
	case float64:
		return time.Duration(item * float64(time.Second)), true
	case string:
		parsed, err := time.ParseDuration(strings.TrimSpace(item))
		return parsed, err == nil
	default:
		return 0, false
	}
}
