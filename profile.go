package dubi

import (
	"fmt"
	"path"
	"strings"
)

// Profile selects registrations for one process role. An empty Include list
// means all entries are eligible. Exclude always wins over Include.
type Profile struct {
	Include  []string
	Exclude  []string
	Profiles []string
}

// NewProfile creates a temporary profile value. It is not added to the
// process-wide profile catalogue until passed to Register.
func NewProfile() *Profile { return &Profile{} }

func (p Profile) matches(name string) bool {
	if len(p.Include) > 0 && !matchesAny(name, p.Include) {
		return false
	}
	return !matchesAny(name, p.Exclude)
}

func matchesAny(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == name {
			return true
		}
		if ok, err := path.Match(pattern, name); err == nil && ok {
			return true
		}
	}
	return false
}

func resolveProfile(r *Registry, name string) (Profile, error) {
	profiles := make(map[string]Profile)
	if r != nil {
		for _, entry := range r.Entries() {
			if profile, ok := asProfile(entry.Value); ok {
				profiles[entry.Name] = profile
			}
		}
	}
	return resolveProfiles(profiles, name)
}

// resolveProfiles expands parent Profiles references and detects cycles before
// a profile is used to filter registrations.
func resolveProfiles(profiles map[string]Profile, name string) (Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Profile{}, nil
	}
	return resolveProfileName(profiles, name, make(map[string]bool))
}

func resolveProfileName(profiles map[string]Profile, name string, visiting map[string]bool) (Profile, error) {
	if visiting[name] {
		return Profile{}, fmt.Errorf("dubi: profile inheritance cycle at %q", name)
	}
	p, ok := profiles[name]
	if !ok {
		return Profile{}, &NotFoundError{Kind: "profile", Name: name}
	}
	visiting[name] = true
	defer delete(visiting, name)
	parents := append([]string(nil), p.Profiles...)
	resolved := Profile{}
	for _, parent := range parents {
		parent = strings.TrimSpace(parent)
		if parent == "" {
			continue
		}
		base, err := resolveProfileName(profiles, parent, visiting)
		if err != nil {
			return Profile{}, err
		}
		resolved = mergeProfiles(resolved, base)
	}
	return mergeProfiles(resolved, p), nil
}

func mergeProfiles(base Profile, overlays ...Profile) Profile {
	merged := Profile{Include: append([]string(nil), base.Include...), Exclude: append([]string(nil), base.Exclude...)}
	for _, overlay := range overlays {
		merged.Include = append(merged.Include, overlay.Include...)
		merged.Exclude = append(merged.Exclude, overlay.Exclude...)
	}
	return merged
}
