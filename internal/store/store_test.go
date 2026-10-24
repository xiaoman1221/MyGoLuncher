package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPortableLayout(t *testing.T) {
	base := t.TempDir()
	paths := PathsFor(base)
	if paths.Base != base {
		t.Fatalf("base = %q", paths.Base)
	}
	if paths.Data != filepath.Join(base, ".minecraft") {
		t.Fatalf("game directory = %q", paths.Data)
	}
	if paths.Root != filepath.Join(base, "MyGoLuncher") {
		t.Fatalf("config directory = %q", paths.Root)
	}
	if paths.Settings != filepath.Join(base, "MyGoLuncher", "settings.json") {
		t.Fatalf("settings path = %q", paths.Settings)
	}
	if paths.Instances != filepath.Join(base, ".minecraft", "instances") {
		t.Fatalf("instances path = %q", paths.Instances)
	}
}

func TestLegacyConfigMigration(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, "MGL")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "settings.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	migrateLegacyConfig(base)
	if _, err := os.Stat(filepath.Join(base, "MyGoLuncher", "settings.json")); err != nil {
		t.Fatalf("legacy config was not migrated: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy directory still exists: %v", err)
	}
}

func TestLanguageDefaultsAndNormalization(t *testing.T) {
	cases := map[string]string{
		"zh-Hant": "zh-TW",
		"zh-HK":   "zh-TW",
		"zh-CN":   "zh-CN",
		"en-US":   "en",
		"ja-JP":   "ja",
		"ko-KR":   "ko",
		"fr-FR":   "zh-CN",
	}
	for input, want := range cases {
		if got := normalizeLanguage(input); got != want {
			t.Fatalf("normalizeLanguage(%q) = %q, want %q", input, got, want)
		}
	}

	paths := PathsFor(t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	// A settings file from before i18n existed has no language fields.
	if err := os.WriteFile(paths.Settings, []byte(`{"playerName":"Steve","downloadWorkers":8}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	settings := st.Settings()
	if !settings.LanguageAuto || settings.Language != "zh-CN" {
		t.Fatalf("legacy language migration failed: %#v", settings)
	}
	if settings.AISearchProvider != "deepseek" || settings.AISearchModel != "deepseek-chat" {
		t.Fatalf("AI defaults = %q / %q", settings.AISearchProvider, settings.AISearchModel)
	}

	settings.LanguageAuto = false
	settings.Language = "en-US"
	settings.VersionIsolation = false
	saved, err := st.SaveSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if saved.LanguageAuto || saved.Language != "en" {
		t.Fatalf("language settings not normalized: %#v", saved)
	}
	if saved.VersionIsolation {
		t.Fatal("version isolation should be saved as disabled")
	}

	reopened, err := Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Settings().LanguageAuto || reopened.Settings().Language != "en" {
		t.Fatalf("language settings not persisted: %#v", reopened.Settings())
	}
	if reopened.Settings().VersionIsolation {
		t.Fatalf("version isolation not persisted: %#v", reopened.Settings())
	}
}

func TestStoreLifecycle(t *testing.T) {
	root := t.TempDir()
	paths := PathsFor(root)
	st, err := Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	if st.Settings().PlayerName == "" {
		t.Fatal("default player name missing")
	}

	settings := st.Settings()
	settings.PlayerName = "Alex"
	settings.DefaultMemoryMB = 6144
	saved, err := st.SaveSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if saved.PlayerName != "Alex" || saved.DefaultMemoryMB != 6144 {
		t.Fatalf("settings not saved: %#v", saved)
	}

	inst, err := st.CreateInstance(Instance{
		Name:          "Test World",
		MCVersion:     "1.21.1",
		Loader:        "fabric",
		LoaderVersion: "0.16.10",
		MemoryMB:      4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.Instances, inst.ID, "mods")); err != nil {
		t.Fatalf("instance directories were not created: %v", err)
	}

	updated, err := st.UpdateInstance(inst.ID, func(i *Instance) error {
		i.Name = "Renamed World"
		i.MemoryMB = 8192
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Renamed World" || updated.MemoryMB != 8192 {
		t.Fatalf("update failed: %#v", updated)
	}
	st.MarkPlayed(inst.ID, 42)
	loaded, ok := st.Instance(inst.ID)
	if !ok || loaded.PlayTimeMinutes != 42 || loaded.LastPlayed.IsZero() {
		t.Fatalf("played metadata not updated: %#v", loaded)
	}

	// Reopen to ensure persistence.
	reopened, err := Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(reopened.Instances()); got != 1 {
		t.Fatalf("expected 1 persisted instance, got %d", got)
	}
	if reopened.Instances()[0].Name != "Renamed World" {
		t.Fatal("instance name did not persist")
	}

	if err := st.DeleteInstance(inst.ID, true); err != nil {
		t.Fatal(err)
	}
	if len(st.Instances()) != 0 {
		t.Fatal("instance was not deleted")
	}
	if _, err := os.Stat(filepath.Join(paths.Instances, inst.ID)); !os.IsNotExist(err) {
		t.Fatal("instance directory was not deleted")
	}
}

func TestSafeName(t *testing.T) {
	valid := []string{"My World", "1.21.1 Fabric", "中文版本"}
	for _, name := range valid {
		if _, err := SafeName(name); err != nil {
			t.Errorf("SafeName(%q) failed: %v", name, err)
		}
	}
	invalid := []string{"", "   ", "a/b", `a\b`, "a:b", "a*b", "a?b", `a"b`}
	for _, name := range invalid {
		if _, err := SafeName(name); err == nil {
			t.Errorf("SafeName(%q) should fail", name)
		}
	}
}

func TestSaveSettingsValidation(t *testing.T) {
	root := t.TempDir()
	st, err := Open(PathsFor(root))
	if err != nil {
		t.Fatal(err)
	}
	settings := st.Settings()
	settings.DefaultMemoryMB = 100
	if _, err := st.SaveSettings(settings); err == nil {
		t.Fatal("expected low memory to be rejected")
	}
	settings.DefaultMemoryMB = 4096
	settings.PlayerName = "a-very-long-player-name-that-exceeds-sixteen"
	saved, err := st.SaveSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(saved.PlayerName)) > 16 {
		t.Fatalf("player name was not truncated: %q", saved.PlayerName)
	}
	settings.DownloadWorkers = 100
	saved, err = st.SaveSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	if saved.DownloadWorkers != 8 {
		t.Fatalf("download workers not normalized: %d", saved.DownloadWorkers)
	}
}

func TestAutomaticMemoryMigration(t *testing.T) {
	base := t.TempDir()
	paths := PathsFor(base)
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Settings, []byte(`{"playerName":"Steve","defaultMemoryMB":4096}`), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Open(paths)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Settings().DefaultMemoryMB; got != 0 {
		t.Fatalf("old 4 GB default should migrate to automatic, got %d", got)
	}
	if st.Settings().SchemaVersion != 3 {
		t.Fatalf("schema version = %d", st.Settings().SchemaVersion)
	}
	if !st.Settings().VersionIsolation {
		t.Fatal("version isolation should default to enabled")
	}
	inst, err := st.CreateInstance(Instance{Name: "Auto", MCVersion: "1.21.1", Loader: "fabric", MemoryMB: 0})
	if err != nil {
		t.Fatal(err)
	}
	if inst.MemoryMB != 0 {
		t.Fatalf("zero memory should mean automatic, got %d", inst.MemoryMB)
	}
	if _, err := st.UpdateInstance(inst.ID, func(i *Instance) error { i.MemoryMB = 0; return nil }); err != nil {
		t.Fatal(err)
	}
}
