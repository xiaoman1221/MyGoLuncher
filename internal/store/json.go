package store

import (
	"encoding/json"
	"path/filepath"

	"MyMinecraftGoLuncher/internal/util"
)

func unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func saveJSON(path string, v any) error {
	return util.WriteJSON(filepath.Clean(path), v)
}
