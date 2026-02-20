package secretmanagersvc

import (
	"fmt"
	"sync"
)

var (
	providers = make(map[string]Provider)
	lock      sync.RWMutex
)

// Register adds a provider to the registry.
// Panics if a provider with the same name is already registered.
// This follows the external-secrets registration pattern.
func Register(name string, provider Provider) {
	lock.Lock()
	defer lock.Unlock()

	if _, exists := providers[name]; exists {
		panic(fmt.Sprintf("provider already registered: %s", name))
	}
	providers[name] = provider
}

// GetProvider retrieves a provider by name from the registry.
// Returns the provider and true if found, nil and false otherwise.
func GetProvider(name string) (Provider, bool) {
	lock.RLock()
	defer lock.RUnlock()

	p, ok := providers[name]
	return p, ok
}

// GetProviders returns a list of all registered provider names.
func GetProviders() []string {
	lock.RLock()
	defer lock.RUnlock()

	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}
