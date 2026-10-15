package dubi

import (
	"reflect"
	"sync"
)

var mounted struct {
	sync.RWMutex
	items  []Module
	frozen bool
}

// Mount attaches a singleton module to the process-wide module set and
// returns its host. Mounting the same module again is a no-op. Once Setup has
// started, mounting any new module panics because it would miss the lifecycle.
func Mount(module Module) Host {
	if module == nil || isNilModule(module) {
		panic("dubi: cannot mount nil module")
	}
	mounted.Lock()
	defer mounted.Unlock()
	if mounted.frozen {
		panic("dubi: cannot mount module after setup")
	}
	for _, existing := range mounted.items {
		if sameModule(existing, module) {
			return moduleHost{}
		}
	}
	mounted.items = append(mounted.items, module)
	return moduleHost{}
}

func isNilModule(module Module) bool {
	value := reflect.ValueOf(module)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func freezeModules() {
	mounted.Lock()
	mounted.frozen = true
	mounted.Unlock()
}

func unfreezeModules() {
	mounted.Lock()
	mounted.frozen = false
	mounted.Unlock()
}

func sameModule(left, right Module) bool {
	leftValue := reflect.ValueOf(left)
	rightValue := reflect.ValueOf(right)
	if !leftValue.IsValid() || !rightValue.IsValid() || leftValue.Type() != rightValue.Type() {
		return false
	}
	if !leftValue.Type().Comparable() {
		return false
	}
	return leftValue.Interface() == rightValue.Interface()
}

func globalModules() []Module {
	mounted.RLock()
	items := append([]Module(nil), mounted.items...)
	mounted.RUnlock()
	return items
}
