package secretstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	keyFileName  = "secrets.key"
	dataFileName = "secrets.dat"
)

type fileStore struct {
	dir      string
	keyPath  string
	dataPath string

	mu     sync.Mutex
	key    []byte
	values map[string]string
	loaded bool
}

// NewFileStore creates an AES-GCM encrypted secret file store. It is used on
// platforms without the Windows registry and by tests.
func NewFileStore(dir string) (Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	// MkdirAll does not tighten permissions on an existing directory.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	store := &fileStore{
		dir:      dir,
		keyPath:  filepath.Join(dir, keyFileName),
		dataPath: filepath.Join(dir, dataFileName),
		values:   map[string]string{},
	}
	if err := store.loadOrCreateKey(); err != nil {
		return nil, err
	}
	return store, nil
}

func (f *fileStore) loadOrCreateKey() error {
	key, err := os.ReadFile(f.keyPath)
	if err == nil {
		if len(key) != 32 {
			// Never replace an unreadable key: doing so would make the
			// existing encrypted data permanently undecryptable.
			return errors.New("密钥文件格式无效")
		}
		if err := os.Chmod(f.keyPath, 0o600); err != nil {
			return err
		}
		f.key = key
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	key = make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return err
	}
	temp, err := os.CreateTemp(f.dir, ".secrets-key-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(key); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, f.keyPath); err != nil {
		return err
	}
	f.key = key
	return nil
}

func (f *fileStore) loadLocked() error {
	if f.loaded {
		return nil
	}
	raw, err := os.ReadFile(f.dataPath)
	if errors.Is(err, os.ErrNotExist) {
		f.loaded = true
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.Chmod(f.dataPath, 0o600); err != nil {
		return err
	}
	block, err := aes.NewCipher(f.key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if len(raw) < gcm.NonceSize() {
		return errors.New("加密数据文件格式无效")
	}
	nonce := raw[:gcm.NonceSize()]
	plain, err := gcm.Open(nil, nonce, raw[gcm.NonceSize():], nil)
	if err != nil {
		return err
	}
	if len(plain) > 0 {
		if err := json.Unmarshal(plain, &f.values); err != nil {
			return err
		}
	}
	f.loaded = true
	return nil
}

func (f *fileStore) saveLocked() error {
	block, err := aes.NewCipher(f.key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	plain, err := json.Marshal(f.values)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	sealed := gcm.Seal(nonce, nonce, plain, nil)
	temp, err := os.CreateTemp(f.dir, ".secrets-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(sealed); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	// On Unix Rename replaces the destination atomically, so a crash can never
	// leave the store without data. Windows cannot replace an existing file,
	// hence the remove+retry fallback (production uses the registry there).
	if err := os.Rename(tempName, f.dataPath); err != nil {
		if removeErr := os.Remove(f.dataPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return err
		}
		if retryErr := os.Rename(tempName, f.dataPath); retryErr != nil {
			return retryErr
		}
	}
	return os.Chmod(f.dataPath, 0o600)
}

func (f *fileStore) Get(name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.loadLocked(); err != nil {
		return "", err
	}
	return f.values[name], nil
}

func (f *fileStore) Set(name, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.loadLocked(); err != nil {
		return err
	}
	if value == "" {
		delete(f.values, name)
	} else {
		f.values[name] = value
	}
	return f.saveLocked()
}

func (f *fileStore) Delete(name string) error {
	return f.Set(name, "")
}
