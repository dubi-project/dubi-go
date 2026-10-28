package dubi

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// logStartup reports readiness only after every module has started. The
// standard Go logger lets applications redirect output with log.SetOutput.
func (rt *Runtime) logStartup(moduleCount int) {
	rt.mu.Lock()
	identity, entries, started := rt.identity, len(rt.entries), rt.startupAt
	rt.mu.Unlock()
	elapsed := time.Duration(0)
	if !started.IsZero() {
		elapsed = time.Since(started).Round(time.Microsecond)
	}
	var message strings.Builder
	fmt.Fprintf(&message, "dubi: ready modules=%d entries=%d elapsed=%s", moduleCount, entries, elapsed)
	for _, field := range []struct{ name, value string }{
		{"tenant", identity.Tenant},
		{"role", identity.Role},
		{"profile", identity.Profile},
		{"node", identity.Node},
	} {
		if field.value != "" {
			fmt.Fprintf(&message, " %s=%q", field.name, field.value)
		}
	}
	log.Print(message.String())
}
