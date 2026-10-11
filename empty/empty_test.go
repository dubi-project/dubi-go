package empty

import (
	"context"
	"testing"

	dubi "github.com/dubi-project/dubi-go"
)

func TestEmptyCanBeRegisteredAndLoaded(t *testing.T) {
	dubi.Register("filter", Filter{})
	dubi.Register("handler", Handler{})
	rt := dubi.NewRuntime()

	ctx, cancel := context.WithCancel(context.Background())
	if err := rt.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rt.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := rt.End(); err != nil {
		t.Fatal(err)
	}
	filters, handlers := Counts()
	if filters == 0 || handlers == 0 {
		t.Fatalf("components were not delivered to the mounted module: filters=%d handlers=%d", filters, handlers)
	}
}
