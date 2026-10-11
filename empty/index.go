// Package empty is a small example of a dubi-go module package.
// Importing the package mounts its singleton Module automatically.
package empty

import dubi "github.com/dubi-project/dubi-go"

var (
	module = &Module{}
	host   dubi.Host
)

func init() {
	host = dubi.Mount(module)
}

// Host returns the host created when this package was mounted.
func Host() dubi.Host { return host }

// Counts reports the number of registered example components in the most
// recently created Runtime instance.
func Counts() (filters, handlers int) {
	mu.RLock()
	instance := last
	mu.RUnlock()
	if instance == nil {
		return 0, 0
	}
	return instance.Counts()
}
