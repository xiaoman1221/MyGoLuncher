package secretstore

import "sync"

type memoryStore struct {
	mu     sync.RWMutex
	values map[string]string
}

// NewMemory creates an in-memory store, useful for tests.
func NewMemory() Store {
	return &memoryStore{values: map[string]string{}}
}

func (m *memoryStore) Get(name string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.values[name], nil
}

func (m *memoryStore) Set(name, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if value == "" {
		delete(m.values, name)
	} else {
		m.values[name] = value
	}
	return nil
}

func (m *memoryStore) Delete(name string) error {
	return m.Set(name, "")
}
