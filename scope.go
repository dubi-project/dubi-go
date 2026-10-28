package dubi

import "strings"

// Scope creates a registration helper that prefixes names with a business
// namespace. It does not change the registered component's Scope or loading rules.
// For example, Scope("user").Register("get", Method{}) registers "user.get".
func Scope(name string) *registerScope {
	return &registerScope{name: normalizeScopeName(name)}
}

type registerScope struct {
	name string
}

// Register registers one or more values under the scoped name. String arguments
// supply the name (the last string wins); all other arguments are registrations.
// Omitting the name registers the values under the scope's own name.
func (s *registerScope) Register(args ...any) {
	name := ""
	for _, arg := range args {
		if value, ok := arg.(string); ok {
			name = value
		}
	}
	target := scopedName(s.name, name)
	for _, value := range args {
		if _, ok := value.(string); !ok {
			Register(target, value)
		}
	}
}

func normalizeScopeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func scopedName(scope, name string) string {
	scope = normalizeScopeName(scope)
	name = normalizeScopeName(name)
	if scope == "" {
		return name
	}
	if name == "" {
		return scope
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "*.") || strings.HasPrefix(name, scope+".") {
		return name
	}
	return scope + "." + name
}
