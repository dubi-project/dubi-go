package web

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestSiteDomainsAliasesAndPrecedence(t *testing.T) {
	for _, fields := range []Map{
		{"domains": "www.example.test"},
		{"domains": []any{"www.example.test"}},
		{"domain": "www.example.test"},
		{"domain": []string{"www.example.test"}},
		{"hosts": []any{"www.example.test"}},
		{"hosts": "old.example.test", "domain": "www.example.test"},
		{"hosts": "old.example.test", "domain": "other.example.test", "domains": "www.example.test"},
	} {
		config, err := parseConfig(Map{"host": "127.0.0.1", "port": 8080, "sites": Map{"www": fields}})
		if err != nil {
			t.Fatal(err)
		}
		if got := config.Sites["www"].Domains; !reflect.DeepEqual(got, []string{"www.example.test"}) {
			t.Fatalf("%v: parsed domains %v", fields, got)
		}
		server := newServerState(defaultServerConfig(config), map[string]struct{}{defaultServerName: {}})
		if site, ok := server.siteForHost("WWW.EXAMPLE.TEST:8080"); !ok || site != "www" {
			t.Fatalf("domain did not match request Host: %q %v", site, ok)
		}
		if _, ok := server.siteForHost("127.0.0.1"); ok {
			t.Fatal("listener address treated as a site domain")
		}
		if server.config.Host != "127.0.0.1" {
			t.Fatal("domain parsing changed listener binding")
		}
		clone := config.Clone()
		clone.Sites["www"].Domains[0] = "changed.test"
		if config.Sites["www"].Domains[0] != "www.example.test" {
			t.Fatal("Clone shares domain slice")
		}
	}
}

func TestAutomaticSiteDomainsDispatchAndURLs(t *testing.T) {
	config, err := parseConfig(Map{"port": 8080, "sites": Map{"api": Map{}, "www": Map{}}})
	if err != nil {
		t.Fatal(err)
	}
	server := newServerState(defaultServerConfig(config), map[string]struct{}{defaultServerName: {}})
	if err := server.register(map[string]Router{
		"api.index": {Uri: "/", Action: func(ctx *Context) { ctx.Text(ctx.Site + " " + ctx.RouteUrl("www.index")) }},
		"www.index": {Uri: "/", Action: func(ctx *Context) { ctx.Text(ctx.Site) }},
	}, nil, nil); err != nil {
		t.Fatal(err)
	}
	for host, want := range map[string]string{
		"api.abcd.com":           "api http://www.abcd.com/",
		"api.test.abcd.com":      "api http://www.test.abcd.com/",
		"api.test.abcd.com:8443": "api http://www.test.abcd.com:8443/",
		"API.TEST.ABCD.COM.":     "api http://www.test.abcd.com/",
		"www.other.test":         "www",
	} {
		request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
		request.Host = host
		response := httptest.NewRecorder()
		server.serveHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.String() != want {
			t.Fatalf("host %q: %d %q, want %q", host, response.Code, response.Body.String(), want)
		}
	}
	for _, host := range []string{"unknown.abcd.com", "abcd.com", "127.0.0.1", "[::1]:8080"} {
		request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
		request.Host = host
		response := httptest.NewRecorder()
		server.serveHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("unmatched host %q: status %d", host, response.Code)
		}
	}
}

func TestServerDomainWithoutSites(t *testing.T) {
	for _, fields := range []Map{
		{}, {"domain": ""}, {"domain": "app.example.test"},
		{"domains": []any{"app.example.test", "alias.example.test"}}, {"domain": "*"},
	} {
		config, err := parseConfig(fields)
		if err != nil {
			t.Fatal(err)
		}
		server := newServerState(defaultServerConfig(config), map[string]struct{}{defaultServerName: {}})
		if _, ok := server.siteForHost("app.example.test:8080"); !ok {
			t.Fatalf("single domain was rejected: %v", fields)
		}
		_, other := server.siteForHost("unknown.test")
		wantOther := len(config.Domains) == 0 || config.Domains[0] == "*"
		if other != wantOther {
			t.Fatalf("domain gate %v: unknown allowed=%v, want %v", fields, other, wantOther)
		}
		if len(config.Domains) > 0 && config.Domains[0] != "*" {
			if got := server.routes.siteURL(&Context{}, "", "/"); got != "http://app.example.test/" {
				t.Fatalf("default URL ignored web.domain: %s", got)
			}
		}
	}
}

func TestSiteDomainsInheritServerBaseAndExplicitSitesOverride(t *testing.T) {
	config, err := parseConfig(Map{
		"host": "127.0.0.1", "port": 8080, "domain": "example.test",
		"sites": Map{"api": Map{}, "www": Map{}, "files": Map{"domain": "cdn.other.test"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := newServerState(defaultServerConfig(config), map[string]struct{}{defaultServerName: {}})
	for host, want := range map[string]string{"api.example.test": "api", "www.example.test": "www", "cdn.other.test": "files"} {
		if site, ok := server.siteForHost(host); !ok || site != want {
			t.Fatalf("%q: site %q matched=%v", host, site, ok)
		}
	}
	for _, host := range []string{"api.other.test", "api.test.example.test", "files.example.test", "files.other.test", "127.0.0.1"} {
		if site, ok := server.siteForHost(host); ok {
			t.Fatalf("%q escaped explicit domains: site %q", host, site)
		}
	}
	ctx := &Context{Site: "api", Host: "api.example.test:8080"}
	if got := server.routes.siteURL(ctx, "www", "/"); got != "http://www.example.test:8080/" {
		t.Fatalf("inherited site URL: %s", got)
	}
	if got := server.routes.siteURL(ctx, "files", "/"); got != "http://cdn.other.test:8080/" {
		t.Fatalf("explicit site URL: %s", got)
	}
	clone := config.Clone()
	clone.Domains[0] = "changed.test"
	if config.Domains[0] != "example.test" {
		t.Fatal("Clone shares server domain list")
	}
}

func TestDomainDefaultsForMultipleServers(t *testing.T) {
	config, err := parseConfig(Map{"domain": "example.test", "servers": Map{
		"public":   Map{"port": 8080, "sites": Map{"api": Map{}}},
		"internal": Map{"port": 8081, "domain": "internal.test", "sites": Map{"api": Map{}}},
		"dynamic":  Map{"port": 8082, "domain": "", "sites": Map{"api": Map{}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]struct{}{"public": {}, "internal": {}, "dynamic": {}}
	public := newServerState(config.Servers["public"], names)
	internal := newServerState(config.Servers["internal"], names)
	dynamic := newServerState(config.Servers["dynamic"], names)
	if _, ok := public.siteForHost("api.example.test"); !ok {
		t.Fatal("server did not inherit web.domain")
	}
	if _, ok := public.siteForHost("api.internal.test"); ok {
		t.Fatal("server selected another server's domain")
	}
	if _, ok := internal.siteForHost("api.internal.test"); !ok {
		t.Fatal("server domain override ignored")
	}
	if _, ok := internal.siteForHost("api.example.test"); ok {
		t.Fatal("inherited domain survived explicit override")
	}
	if _, ok := dynamic.siteForHost("api.test.abcd.com"); !ok {
		t.Fatal("empty server domain did not restore automatic matching")
	}
}

func TestInvalidDomainConfigurationIsRejected(t *testing.T) {
	for _, section := range []Map{
		{"domain": false}, {"domains": 3}, {"domains": []any{"example.test", false}},
		{"servers": Map{"api": Map{"domain": 3}}},
		{"sites": Map{"api": Map{"domain": false}}},
		{"sites": Map{"api": Map{"domains": []any{"api.test", 3}}}},
	} {
		if _, err := parseConfig(section); err == nil {
			t.Errorf("invalid domains became automatic matching: %v", section)
		}
	}
}

func TestExplicitDomainsAndCatchAllTakeExpectedPriority(t *testing.T) {
	config, err := parseConfig(Map{"sites": Map{
		"api":      Map{},
		"special":  Map{"domain": "api.specific.test"},
		"wildcard": Map{"domain": "*.wildcard.test"},
		"fallback": Map{"domain": "*"},
		"127":      Map{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	server := newServerState(defaultServerConfig(config), map[string]struct{}{defaultServerName: {}})
	for host, want := range map[string]string{
		"api.specific.test": "special", "api.wildcard.test": "wildcard",
		"api.other.test": "api", "unknown.test": "fallback", "127.0.0.1": "fallback",
	} {
		if site, ok := server.siteForHost(host); !ok || site != want {
			t.Fatalf("%q selected %q matched=%v, want %q", host, site, ok, want)
		}
	}
}

func TestDomainsMatchWildcardsAndGenerateSiteURLs(t *testing.T) {
	config, err := parseConfig(Map{"port": 8080, "sites": Map{
		"www": Map{"domains": []any{"*.example.test", "www.example.test"}},
		"api": Map{"domain": "api.example.test"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	server := newServerState(defaultServerConfig(config), map[string]struct{}{defaultServerName: {}})
	for host, want := range map[string]string{"api.example.test": "api", "tenant.example.test": "www"} {
		if site, ok := server.siteForHost(host); !ok || site != want {
			t.Fatalf("%q matched %q %v, want %q", host, site, ok, want)
		}
	}
	if _, ok := server.siteForHost("unknown.test"); ok {
		t.Fatal("unknown domain matched a site")
	}
	ctx := &Context{Site: "www", Host: "tenant.example.test:8080"}
	if got := server.routes.siteURL(ctx, "www", "/asset"); got != "http://tenant.example.test:8080/asset" {
		t.Fatalf("current request domain lost: %s", got)
	}
	if got := server.routes.siteURL(ctx, "api", "/user"); got != "http://api.example.test:8080/user" {
		t.Fatalf("cross-site URL did not use configured domain: %s", got)
	}
	ctx.Site = "api"
	if got := server.routes.siteURL(ctx, "www", "/asset"); got != "http://www.example.test:8080/asset" {
		t.Fatalf("wildcard used as URL hostname: %s", got)
	}
}
