package dubi

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

func TestAuthTokenRoundTripAndRevoke(t *testing.T) {
	rt := testRuntime()
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	raw, err := rt.SignToken(Token{
		Auth:    true,
		Kind:    "user",
		Payload: Map{"id": "u-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := rt.VerifyToken(raw)
	if err != nil {
		t.Fatal(err)
	}
	if token.ID == "" || !token.Auth || token.Kind != "user" || token.Payload["id"] != "u-1" {
		t.Fatalf("token = %#v", token)
	}
	if err := rt.RevokeToken(token.ID, token.Expires); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.VerifyToken(raw); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("verify after revoke = %v, want %v", err, ErrTokenRevoked)
	}
}

func TestAuthMetaUsesProcessAndTracksIdentity(t *testing.T) {
	rt := testRuntime()
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	meta := NewMeta()
	raw := meta.Auth(Map{"id": "u-2"})
	if raw == "" || !meta.Signed() || !meta.Authed() || meta.Kind() != "" {
		t.Fatalf("meta auth state: token=%q signed=%v authed=%v kind=%q", raw, meta.Signed(), meta.Authed(), meta.Kind())
	}
	if got := meta.Payload()["id"]; got != "u-2" {
		t.Fatalf("payload id = %#v", got)
	}
	if err := meta.RevokeCurrentToken(); err != nil {
		t.Fatal(err)
	}
	if _, err := meta.VerifyToken(raw); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("verify revoked token = %v", err)
	}
}

func TestAuthExpiredToken(t *testing.T) {
	rt := testRuntime()
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	raw, err := rt.SignToken(Token{Expires: time.Now().Add(-time.Second).Unix()})
	if err == nil || raw != "" {
		t.Fatalf("expired issue = %q, %v", raw, err)
	}
}

func TestAuthUsesProcessSecret(t *testing.T) {
	rt := testRuntime()
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())
	raw, err := rt.SignToken(Token{Auth: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.VerifyToken(raw); err != nil {
		t.Fatalf("process verify = %v", err)
	}
}
