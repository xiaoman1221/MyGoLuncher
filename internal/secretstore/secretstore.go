// Package secretstore persists launcher secrets outside the portable MGL
// folder. Windows uses the per-user registry protected with DPAPI; other
// platforms use an AES-GCM encrypted file in the user config directory.
package secretstore

// Store is a small named-secret storage abstraction.
type Store interface {
	// Get returns an empty string when the named secret does not exist.
	Get(name string) (string, error)
	// Set stores a secret. An empty value deletes it.
	Set(name, value string) error
	// Delete removes a secret if it exists.
	Delete(name string) error
}
