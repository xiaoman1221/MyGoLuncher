//go:build windows

package license

import "golang.org/x/sys/windows/registry"

func platformMachineGUID() string {
	const path = `SOFTWARE\Microsoft\Cryptography`
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, err := key.GetStringValue("MachineGuid")
	if err != nil {
		return ""
	}
	return value
}
