package main

import (
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
)

const authCacheTTL = 6 * time.Hour

type cachedAuth struct {
	auth      *authn.AuthConfig
	expiresAt time.Time
}

// cachedKeychain wraps an authn.Keychain and caches resolved credentials
// per registry for a configurable TTL. This prevents creating new AWS SDK
// sessions and ECR API calls on every image verification request, which
// was causing goroutine/HTTP transport leaks leading to OOM.
type cachedKeychain struct {
	inner authn.Keychain
	mu    sync.RWMutex
	cache map[string]cachedAuth
}

func NewCachedKeychain(inner authn.Keychain) authn.Keychain {
	return &cachedKeychain{
		inner: inner,
		cache: make(map[string]cachedAuth),
	}
}

func (c *cachedKeychain) Resolve(target authn.Resource) (authn.Authenticator, error) {
	key := target.RegistryStr()

	c.mu.RLock()
	if entry, ok := c.cache[key]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return authn.FromConfig(*entry.auth), nil
	}
	c.mu.RUnlock()

	// Cache miss or expired — resolve from inner keychain
	auth, err := c.inner.Resolve(target)
	if err != nil {
		return nil, err
	}

	// Extract the auth config to cache it
	authConfig, err := auth.Authorization()
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.cache[key] = cachedAuth{
		auth:      authConfig,
		expiresAt: time.Now().Add(authCacheTTL),
	}
	c.mu.Unlock()

	return authn.FromConfig(*authConfig), nil
}
