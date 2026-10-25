package license

import (
	"errors"
	"strings"
)

// ErrProductNotConfigured marks a community build. The launcher stays fully
// usable for free features; paid features and activation require an official
// build made with private product credentials.
var ErrProductNotConfigured = errors.New("商业授权服务未配置")

// These values are injected only into official builds, for example:
//
//	-ldflags "-X MyMinecraftGoLuncher/internal/license.ProductAPIKey=... //	          -X MyMinecraftGoLuncher/internal/license.ProductServerURL=https://..."
//
// They are deliberately absent from the public repository. The LicensingGo
// product key is a client-distributed identifier, not a server master secret.
var (
	ProductAPIKey    = ""
	ProductServerURL = ""
	ProductName      = "MyGoLuncher"
	ProductSlug      = "MyGo"
)

// ServerURL is an optional staging override used by tests and release builds.
var ServerURL = ""

type productConfig struct {
	APIKey string
	Server string
	Name   string
	Slug   string
}

func loadProductConfig() (productConfig, error) {
	config := productConfig{
		APIKey: strings.TrimSpace(ProductAPIKey),
		Server: strings.TrimSpace(ProductServerURL),
		Name:   strings.TrimSpace(ProductName),
		Slug:   strings.TrimSpace(ProductSlug),
	}
	if config.APIKey == "" || config.Server == "" {
		return config, ErrProductNotConfigured
	}
	return config, nil
}

func productAPIKey() (string, error) {
	config, err := loadProductConfig()
	if err != nil {
		return "", err
	}
	return config.APIKey, nil
}

// ProductInfo returns the configured product name and identifier without
// exposing the API key.
func ProductInfo() (name, slug string) {
	config, err := loadProductConfig()
	if err != nil {
		return "MyGoLuncher", "MyGo"
	}
	return config.Name, config.Slug
}
