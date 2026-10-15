package dubi

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Identity describes the process configuration scope.
type Identity struct {
	Tenant  string
	Role    string
	Profile string
	Node    string
}

func WithTenant(tenant string) RuntimeOption {
	return func(rt *Runtime) { rt.identity.Tenant = strings.TrimSpace(tenant) }
}

func WithRole(role string) RuntimeOption {
	return func(rt *Runtime) { rt.identity.Role = strings.TrimSpace(role) }
}

func WithProfile(profile string) RuntimeOption {
	return func(rt *Runtime) {
		rt.identity.Profile = strings.TrimSpace(profile)
		rt.profileSet = true
	}
}

func WithNode(node string) RuntimeOption {
	return func(rt *Runtime) {
		rt.identity.Node = strings.TrimSpace(node)
		rt.nodeSet = rt.identity.Node != ""
	}
}

var processIdentity struct {
	sync.Once
	value Identity
}

var processNode struct {
	sync.Once
	value string
}

var processRequest struct {
	sync.Once
	value string
}

var requestSequence atomic.Uint64

// newRequestID uses one random process prefix and an atomic sequence. The
// random prefix separates processes while the sequence avoids a crypto/rand
// call on every request.
func newRequestID() string {
	processRequest.Do(func() { processRequest.value = newUniqueID("req") })
	var buffer [64]byte
	value := buffer[:0]
	value = append(value, processRequest.value...)
	value = append(value, '-')
	value = strconv.AppendUint(value, requestSequence.Add(1), 36)
	return string(value)
}

func defaultNodeID() string {
	processNode.Do(func() {
		processNode.value = newNodeID()
	})
	return processNode.value
}

func defaultIdentity() Identity {
	processIdentity.Do(func() {
		processIdentity.value = parseIdentity(os.Args[1:], os.Getenv)
	})
	return processIdentity.value
}

func parseIdentity(args []string, getenv func(string) string) Identity {
	value := func(name string) string {
		if value, ok := commandValue(args, name); ok {
			return strings.TrimSpace(value)
		}
		return strings.TrimSpace(getenv("DUBI_" + strings.ToUpper(name)))
	}
	return Identity{Tenant: value("tenant"), Role: value("role"), Profile: value("profile"), Node: value("node")}
}

func commandValue(args []string, name string) (string, bool) {
	prefix := "--" + name
	value, found := "", false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if arg == prefix && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			value, found = args[i+1], true
			i++
		}
		if strings.HasPrefix(arg, prefix+"=") {
			value, found = strings.TrimPrefix(arg, prefix+"="), true
		}
	}
	return value, found
}

func newNodeID() string {
	return newUniqueID("node")
}

func newUniqueID(prefix string) string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic("dubi: cannot generate unique ID: " + err.Error())
	}
	return prefix + "-" + hex.EncodeToString(buf)
}
