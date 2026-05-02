package persistent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tidwall/buntdb"
)

func TestOpenDB(t *testing.T) {
	testName := "mcp-test-brainstorm-" + t.Name()

	db, err := OpenDB(testName)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	// Verify it works
	err = db.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set("key", "value", nil)
		return err
	})
	if err != nil {
		t.Errorf("DB update failed: %v", err)
	}

	err = db.View(func(tx *buntdb.Tx) error {
		val, err := tx.Get("key")
		if err != nil {
			return err
		}
		if val != "value" {
			t.Errorf("expected value, got %q", val)
		}
		return nil
	})
	if err != nil {
		t.Errorf("DB view failed: %v", err)
	}

	// Cleanup
	cacheDir, _ := os.UserCacheDir()
	os.RemoveAll(filepath.Join(cacheDir, testName))
}
