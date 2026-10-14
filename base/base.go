// Package base contains the small, dependency-free types shared by dubi-go
// packages. It deliberately has no runtime, protocol, or capability logic.
package base

// Any is kept as a named alias for documentation and compatibility with the
// dynamic APIs used throughout dubi-go. It is exactly the language predeclared
// any type.
type Any = any

// Map is the common dynamic object passed between framework boundaries such as
// Method, Service, HTTP, Queue, Event, and Bus calls.
type Map map[string]Any

// List is a dynamic array used where naming the shape improves readability.
type List []Any

func (m Map) Get(key string) (Any, bool) {
	if m == nil {
		return nil, false
	}
	v, ok := m[key]
	return v, ok
}

func (m Map) Set(key string, value Any) Map {
	if m == nil {
		m = make(Map)
	}
	m[key] = value
	return m
}

func (m Map) Clone() Map {
	if m == nil {
		return nil
	}
	copyOf := make(Map, len(m))
	for key, value := range m {
		copyOf[key] = value
	}
	return copyOf
}
