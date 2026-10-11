package dubi

import "path"

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
	if name == "" {
		return Profile{}, nil
	}
	v, ok := r.Lookup(name)
	if !ok {
		return Profile{}, &NotFoundError{Kind: "profile", Name: name}
	}
	p, ok := v.(Profile)
	if !ok {
		if pp, ok := v.(*Profile); ok && pp != nil {
			p = *pp
		} else {
			return Profile{}, &TypeError{Name: name, Want: "dubi.Profile", Got: v}
		}
	}
	for _, parent := range p.Profiles {
		base, err := resolveProfile(r, parent)
		if err != nil {
			return Profile{}, err
		}
		p = mergeProfiles(base, p)
	}
	return p, nil
}

func mergeProfiles(base Profile, overlays ...Profile) Profile {
	merged := Profile{Include: append([]string(nil), base.Include...), Exclude: append([]string(nil), base.Exclude...)}
	for _, overlay := range overlays {
		merged.Include = append(merged.Include, overlay.Include...)
		merged.Exclude = append(merged.Exclude, overlay.Exclude...)
	}
	return merged
}
