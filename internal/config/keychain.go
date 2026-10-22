package config

import "github.com/zalando/go-keyring"

// keychainService is the system keychain service every termwise entry is
// stored under.
const keychainService = "termwise"

// DefaultKeychainEntry returns the conventional keychain entry name for a provider.
func DefaultKeychainEntry(providerName string) string {
	return "termwise-" + providerName
}

// StoreKeychain writes an API key into the system keychain under the termwise
// service, at the provider's default entry.
func StoreKeychain(providerName, apiKey string) error {
	return StoreKeychainEntry(DefaultKeychainEntry(providerName), apiKey)
}

// StoreKeychainEntry writes an API key into the system keychain under the
// termwise service.
func StoreKeychainEntry(entry, apiKey string) error {
	return keyring.Set(keychainService, entry, apiKey)
}
