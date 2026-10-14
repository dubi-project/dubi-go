package dubi

import (
	"os"
	"strings"

	. "github.com/dubi-project/dubi-go/base"
)

// Bootstrap contains the information needed before a Runtime can connect to
// Dubi and load its full configuration.
type Bootstrap struct {
	Endpoints []string
	Auth      Map
}

func WithEndpoint(endpoint string) RuntimeOption {
	return func(rt *Runtime) {
		if endpoint = strings.TrimSpace(endpoint); endpoint != "" {
			rt.bootstrap.Endpoints = []string{endpoint}
		}
	}
}

func WithEndpoints(endpoints ...string) RuntimeOption {
	return func(rt *Runtime) {
		rt.bootstrap.Endpoints = cleanEndpoints(endpoints)
	}
}

func defaultBootstrap() Bootstrap {
	commandEndpoints, _ := commandValue(os.Args[1:], "endpoints")
	return Bootstrap{
		Endpoints: parseEndpoints(commandEndpoints, os.Getenv("DUBI_ENDPOINTS")),
		Auth:      parseAuthEnvironment(os.Getenv),
	}
}

func cleanEndpoints(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, endpoint := range strings.Split(value, ",") {
			if endpoint = strings.TrimSpace(endpoint); endpoint != "" {
				result = append(result, endpoint)
			}
		}
	}
	return result
}

func parseEndpoints(command, environment string) []string {
	if strings.TrimSpace(command) != "" {
		return cleanEndpoints([]string{command})
	}
	return cleanEndpoints([]string{environment})
}

func parseAuthEnvironment(getenv func(string) string) Map {
	auth := Map{}
	for _, key := range []string{"TYPE", "TOKEN", "USERNAME", "PASSWORD", "CERT", "KEY", "CA"} {
		if value := strings.TrimSpace(getenv("DUBI_AUTH_" + key)); value != "" {
			auth[strings.ToLower(key)] = value
		}
	}
	return auth
}
