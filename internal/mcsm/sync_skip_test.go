package mcsm

import "testing"

func TestSkipRollbackModsFromServerSync(t *testing.T) {
	for _, name := range []string{"sodium.jar.old", "sodium.jar.old.1", "Sodium.JAR.OLD", "sodium.jar.dis"} {
		if !skipSyncName(name) {
			t.Errorf("rollback file %q should not be synced", name)
		}
	}
	if skipSyncName("sodium.jar") {
		t.Fatal("normal jar should still be synced")
	}
}
