package web

import (
	"fmt"
	"net"
	"sort"
	"strings"

	. "github.com/dubi-project/dubi-go/base"
)

// configuredDomains distinguishes an omitted field (inherit) from an empty
// field (use automatic matching). Invalid values must not become unrestricted.
func configuredDomains(section Map, names ...string) ([]string, bool, error) {
	for _, name := range names {
		value, present := section[name]
		if !present {
			continue
		}
		switch item := value.(type) {
		case nil:
			return nil, true, nil
		case string:
			return stringList(item), true, nil
		case []string:
			return cleanStrings(item), true, nil
		case []any:
			values := make([]string, 0, len(item))
			for _, value := range item {
				text, ok := value.(string)
				if !ok {
					return nil, true, fmt.Errorf("%s must contain only strings", name)
				}
				values = append(values, text)
			}
			return cleanStrings(values), true, nil
		default:
			return nil, true, fmt.Errorf("%s must be a string or a list of strings", name)
		}
	}
	return nil, false, nil
}

// effectiveSiteDomains supplies the same domain defaults to dispatch and URL
// generation. Explicit site domains override server domain defaults.
func effectiveSiteDomains(config ServerConfig, name string, site SiteConfig) ([]string, bool) {
	domains := append([]string(nil), site.Domains...)
	automatic := false
	if len(domains) == 0 {
		automatic = len(config.Domains) == 0
		for _, domain := range config.Domains {
			domain = strings.TrimSpace(domain)
			if domain == "*" {
				automatic = true
				continue
			}
			if domain == "" {
				continue
			}
			// A wildcard server domain still identifies the base domain.
			domain = strings.TrimPrefix(domain, "*.")
			domains = append(domains, strings.ToLower(name)+"."+domain)
		}
	}
	return append(domains, site.Aliases...), automatic
}

func compileSiteHosts(config ServerConfig) (map[string]string, []siteHostPattern, map[string]string) {
	exact := make(map[string]string)
	wildcards := make([]siteHostPattern, 0)
	automatic := make(map[string]string)
	add := func(name string, candidates []string) {
		for _, candidate := range candidates {
			candidate = normalizeHostPattern(candidate)
			if candidate == "" {
				continue
			}
			if candidate == "*" {
				wildcards = append(wildcards, siteHostPattern{site: name})
				continue
			}
			if strings.HasPrefix(candidate, "*.") {
				wildcards = append(wildcards, siteHostPattern{suffix: candidate[1:], site: name})
				continue
			}
			exact[candidate] = name
		}
	}
	if len(config.Sites) == 0 {
		add("", config.Domains)
	} else {
		for _, name := range sortedKeys(config.Sites) {
			domains, infer := effectiveSiteDomains(config, name, config.Sites[name])
			add(name, domains)
			if infer {
				automatic[strings.ToLower(name)] = name
			}
		}
	}
	sort.SliceStable(wildcards, func(left, right int) bool {
		if len(wildcards[left].suffix) != len(wildcards[right].suffix) {
			return len(wildcards[left].suffix) > len(wildcards[right].suffix)
		}
		return wildcards[left].site < wildcards[right].site
	})
	return exact, wildcards, automatic
}

func normalizeHostPattern(raw string) string {
	host := strings.ToLower(strings.TrimSpace(raw))
	if host == "" || host == "*" {
		return host
	}
	if strings.HasPrefix(host, "*.") {
		return "*." + strings.TrimSuffix(strings.TrimSpace(host[2:]), ".")
	}
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	return strings.TrimSuffix(host, ".")
}

func (s *serverState) siteForHost(rawHost string) (string, bool) {
	if len(s.config.Sites) == 0 && len(s.config.Domains) == 0 {
		return "", true
	}
	host := normalizeHostPattern(rawHost)
	if site, ok := s.exactSites[host]; ok {
		return site, true
	}
	for _, pattern := range s.wildcardSites {
		if pattern.suffix != "" && strings.HasSuffix(host, pattern.suffix) {
			return pattern.site, true
		}
	}
	// Only sites without explicit domains can use automatic subdomain routing.
	// IP addresses are never split into numeric site names.
	if net.ParseIP(strings.Trim(host, "[]")) == nil {
		label, _, _ := strings.Cut(host, ".")
		if site, ok := s.automaticSites[label]; ok {
			return site, true
		}
	}
	for _, pattern := range s.wildcardSites {
		if pattern.suffix == "" {
			return pattern.site, true
		}
	}
	return "", false
}
