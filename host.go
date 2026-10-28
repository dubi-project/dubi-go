package dubi

import . "github.com/dubi-project/dubi-go/base"

// Host is the narrow communication surface given to a mounted module. It
// lets a module package interact with the host package without creating an
// import cycle in the opposite direction.
type Host interface {
	RegisterLocal(name string, value any)
	Lookup(name string) (any, bool)
	Toggle(name string, values ...Map)
	SyncToggle(name string, values ...Map)
}

type moduleHost struct{}

func (moduleHost) RegisterLocal(name string, value any)  { Register(name, value) }
func (moduleHost) Lookup(name string) (any, bool)        { return Lookup(name) }
func (moduleHost) Toggle(name string, values ...Map)     { Toggle(name, values...) }
func (moduleHost) SyncToggle(name string, values ...Map) { SyncToggle(name, values...) }
