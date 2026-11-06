package dubi

import (
	"fmt"

	. "github.com/dubi-project/dubi-go/base"
)

// Host is the narrow communication surface given to a mounted module. It
// lets a module package interact with the host package without creating an
// import cycle in the opposite direction.
type Host interface {
	RegisterLocal(name string, value any)
	Lookup(name string) (any, bool)
	Toggle(name string, values ...Map)
	SyncToggle(name string, values ...Map)
	// Services returns the Profile-selected catalog, available during Setup.
	Services() []ServiceInfo
	// InvokeLocalService never routes through BUS or executes a Method.
	InvokeLocalService(meta *Meta, name string, value Map, delivery ...BusDelivery) (Map, Res, bool)
}

type moduleHost struct{}

func (moduleHost) RegisterLocal(name string, value any)  { Register(name, value) }
func (moduleHost) Lookup(name string) (any, bool)        { return Lookup(name) }
func (moduleHost) Toggle(name string, values ...Map)     { Toggle(name, values...) }
func (moduleHost) SyncToggle(name string, values ...Map) { SyncToggle(name, values...) }

func (moduleHost) Services() []ServiceInfo { return serviceCatalog() }

func (moduleHost) InvokeLocalService(meta *Meta, name string, value Map, delivery ...BusDelivery) (data Map, result Res, found bool) {
	// Track incoming deliveries as well as outgoing calls. They must stop before
	// another module can close the resources used by the business Action.
	_, local, done, err := bus.acquire(meta)
	if err != nil {
		return nil, ErrorResult(err), false
	}
	defer done()
	defer func() {
		if recovered := recover(); recovered != nil {
			data, result, found = nil, ErrorResult(fmt.Errorf("dubi: service %q panic: %v", name, recovered)), true
		}
	}()
	local.attempt = meta.Attempt()
	if len(delivery) > 0 {
		if delivery[0].ID != "" {
			local.id = delivery[0].ID
		}
		local.attempt = delivery[0].Attempt
	}
	return DefaultRuntime().invokeLocalService(local, name, value)
}
