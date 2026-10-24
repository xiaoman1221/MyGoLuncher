package mojang

import (
	"fmt"
	"os"
	"path/filepath"

	"MyMinecraftGoLuncher/internal/util"
)

// MaterializeAssets recreates a logical asset tree from the hashed object
// store. This is required by legacy indexes using virtual assets or a
// resources directory.
func MaterializeAssets(catalog AssetCatalog, objectRoot, dest string) error {
	marker := filepath.Join(dest, ".minecraft-go-complete")
	if util.Exists(marker) {
		return nil
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for name, object := range catalog.Objects {
		if len(object.Hash) < 2 {
			continue
		}
		target, err := util.SafeJoin(dest, name)
		if err != nil {
			continue
		}
		if util.Exists(target) {
			continue
		}
		source := filepath.Join(objectRoot, object.Hash[:2], object.Hash)
		if !util.Exists(source) {
			return fmt.Errorf("资源文件缺失: %s", object.Hash)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.Link(source, target); err != nil {
			if err := util.CopyFile(source, target); err != nil {
				return err
			}
		}
	}
	return os.WriteFile(marker, []byte("ok"), 0o644)
}
