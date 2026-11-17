// Package secrets keeps credentials out of GitOwl's preference file. On
// macOS the Keychain holds them, through /usr/bin/security with the secret
// written to its standard input rather than its arguments.
package secrets

import (
	"errors"
	"sync"

	"github.com/zalando/go-keyring"
)

// Service names GitOwl's Keychain items.
const Service = "dev.gitowl.app"

// APIKeyAccount is the account of the AI service's API key.
const APIKeyAccount = "ai-api-key"

// Store reads and writes secrets. Get returns "" when none is saved.
type Store interface {
	Get(account string) (string, error)
	Set(account, secret string) error
	Delete(account string) error
}

// Keychain stores secrets in the user's login keychain.
type Keychain struct{}

func (Keychain) Get(account string) (string, error) {
	s, err := keyring.Get(Service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return s, err
}

func (Keychain) Set(account, secret string) error { return keyring.Set(Service, account, secret) }

func (Keychain) Delete(account string) error {
	err := keyring.Delete(Service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// Memory keeps secrets in memory, for tests.
type Memory struct {
	mu sync.Mutex
	m  map[string]string
	// Err, when set, fails every call.
	Err error
}

func (s *Memory) Get(account string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return "", s.Err
	}
	return s.m[account], nil
}

func (s *Memory) Set(account, secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[account] = secret
	return nil
}

func (s *Memory) Delete(account string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	delete(s.m, account)
	return nil
}
