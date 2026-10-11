package dubi

import "sync"

var mounted struct {
	sync.RWMutex
	items []Module
}

// Mount attaches a module definition to the process-wide module set and
// returns its host. The definition creates one Instance per Runtime.
func Mount(module Module) Host {
	if module == nil {
		panic("dubi: cannot mount nil module")
	}
	mounted.Lock()
	mounted.items = append(mounted.items, module)
	mounted.Unlock()
	return moduleHost{}
}

func globalModules() []Module {
	mounted.RLock()
	items := append([]Module(nil), mounted.items...)
	mounted.RUnlock()
	return items
}
