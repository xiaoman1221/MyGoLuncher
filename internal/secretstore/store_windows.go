//go:build windows

package secretstore

// New returns the per-user registry secret store on Windows.
func New() (Store, error) {
	return newRegistryStore(), nil
}
