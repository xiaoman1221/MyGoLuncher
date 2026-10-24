//go:build windows

package secretstore

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

const (
	registryRoot            = `Software\MyGoLuncher\Secrets`
	cryptProtectUIForbidden = 0x1
)

var (
	crypt32                = syscall.NewLazyDLL("crypt32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procCryptProtectData   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	procLocalFree          = kernel32.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

type registryStore struct{}

func newRegistryStore() Store {
	return &registryStore{}
}

func (r *registryStore) Get(name string) (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryRoot, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer key.Close()
	encrypted, _, err := key.GetBinaryValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(encrypted) == 0 {
		return "", nil
	}
	plain, err := unprotectData(encrypted)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (r *registryStore) Set(name, value string) error {
	if value == "" {
		return r.Delete(name)
	}
	encrypted, err := protectData([]byte(value))
	if err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, registryRoot, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetBinaryValue(name, encrypted)
}

func (r *registryStore) Delete(name string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryRoot, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()
	err = key.DeleteValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func protectData(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	input := dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
	var output dataBlob
	ok, _, callErr := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&input)),
		0,
		0,
		0,
		0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&output)),
	)
	if ok == 0 {
		return nil, fmt.Errorf("DPAPI 加密失败: %v", callErr)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(output.pbData)))
	if output.cbData == 0 || output.pbData == nil {
		return nil, errors.New("DPAPI 加密结果为空")
	}
	return append([]byte(nil), unsafe.Slice(output.pbData, int(output.cbData))...), nil
}

func unprotectData(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	input := dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
	var output dataBlob
	ok, _, callErr := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&input)),
		0,
		0,
		0,
		0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&output)),
	)
	if ok == 0 {
		return nil, fmt.Errorf("DPAPI 解密失败: %v", callErr)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(output.pbData)))
	if output.cbData == 0 || output.pbData == nil {
		return []byte{}, nil
	}
	return append([]byte(nil), unsafe.Slice(output.pbData, int(output.cbData))...), nil
}
