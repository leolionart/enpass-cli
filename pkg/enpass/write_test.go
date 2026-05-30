package enpass

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
)

func copyTestVault(t *testing.T) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "enpass-test-*")
	if err != nil {
		t.Fatalf("could not create temp dir: %v", err)
	}

	// Copy vault files
	srcDB, _ := os.ReadFile("../../test/vault.enpassdb")
	srcJSON, _ := os.ReadFile("../../test/vault.json")

	os.WriteFile(filepath.Join(tmpDir, "vault.enpassdb"), srcDB, 0600)
	os.WriteFile(filepath.Join(tmpDir, "vault.json"), srcJSON, 0600)

	return tmpDir
}

func TestVault_CreateEntry(t *testing.T) {
	tmpDir := copyTestVault(t)
	defer os.RemoveAll(tmpDir)

	vault, err := NewVault(tmpDir, logrus.ErrorLevel)
	if err != nil {
		t.Fatalf("vault initialization failed: %v", err)
	}
	defer vault.Close()

	credentials := &VaultCredentials{Password: testPassword}
	if err := vault.Open(credentials); err != nil {
		t.Skipf("skipping test: could not open vault (environmental issue): %v", err)
	}

	entry := &EntryData{
		Title:    "Test Entry",
		Username: "testuser@example.com",
		Password: "testpassword123",
		URL:      "https://example.com",
		Notes:    "Test notes",
		Category: "Login",
	}

	uuid, err := vault.CreateEntry(entry)
	if err != nil {
		t.Fatalf("CreateEntry failed: %v", err)
	}

	if uuid == "" {
		t.Error("CreateEntry returned empty UUID")
	}

	// Verify we can retrieve the entry
	cards, err := vault.GetEntries("password", []string{"Test Entry"})
	if err != nil {
		t.Fatalf("GetEntries failed: %v", err)
	}

	if len(cards) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cards))
	}

	if cards[0].Title != "Test Entry" {
		t.Errorf("expected title 'Test Entry', got %q", cards[0].Title)
	}

	// Verify password decrypts correctly
	decrypted, err := cards[0].Decrypt()
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != "testpassword123" {
		t.Errorf("expected password 'testpassword123', got %q", decrypted)
	}

	// URL is stored in an item field, not the title/subtitle columns.
	cards, err = vault.GetEntries("password", []string{"example.com"})
	if err != nil {
		t.Fatalf("GetEntries by URL field failed: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("expected 1 entry by URL field, got %d", len(cards))
	}

	fields, err := vault.GetPublicFields(uuid)
	if err != nil {
		t.Fatalf("GetPublicFields failed: %v", err)
	}
	if len(fields) == 0 {
		t.Fatal("expected public fields")
	}

	assertCreatedEntryMetadata(t, vault, uuid)
}

func assertCreatedEntryMetadata(t *testing.T, vault *Vault, entryUUID string) {
	t.Helper()

	var metaUpdatedAt, updatedAt int64
	var template, icon string
	if err := vault.db.QueryRow(`
		SELECT meta_updated_at, updated_at, template, icon
		FROM item
		WHERE uuid = ?
	`, entryUUID).Scan(&metaUpdatedAt, &updatedAt, &template, &icon); err != nil {
		t.Fatalf("could not read created item metadata: %v", err)
	}
	if metaUpdatedAt <= 0 {
		t.Errorf("expected item meta_updated_at to be set, got %d", metaUpdatedAt)
	}
	if updatedAt <= 0 {
		t.Errorf("expected item updated_at to be set, got %d", updatedAt)
	}
	if template != "login.default" {
		t.Errorf("expected template login.default, got %q", template)
	}
	if icon == "" {
		t.Error("expected icon metadata to be set")
	}

	rows, err := vault.db.Query(`
		SELECT type, item_field_uid, historical, updated_at, value_updated_at,
		       orde, wearable, history, hash, algo_version, extra
		FROM itemfield
		WHERE item_uuid = ? AND type IN ('password', 'username', 'url')
	`, entryUUID)
	if err != nil {
		t.Fatalf("could not read created item fields: %v", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var fieldType, history, hash, extra string
		var fieldUID, historical, fieldUpdatedAt, valueUpdatedAt, order, wearable, algoVersion int64
		if err := rows.Scan(
			&fieldType, &fieldUID, &historical, &fieldUpdatedAt, &valueUpdatedAt,
			&order, &wearable, &history, &hash, &algoVersion, &extra,
		); err != nil {
			t.Fatalf("could not scan created item field metadata: %v", err)
		}
		seen[fieldType] = true
		if fieldUID <= 0 {
			t.Errorf("expected %s item_field_uid to be set, got %d", fieldType, fieldUID)
		}
		if historical != 1 {
			t.Errorf("expected %s historical=1, got %d", fieldType, historical)
		}
		if fieldUpdatedAt <= 0 || valueUpdatedAt <= 0 {
			t.Errorf("expected %s timestamps to be set, got updated_at=%d value_updated_at=%d", fieldType, fieldUpdatedAt, valueUpdatedAt)
		}
		if order <= 0 {
			t.Errorf("expected %s orde to be set, got %d", fieldType, order)
		}
		if wearable != 0 {
			t.Errorf("expected %s wearable=0, got %d", fieldType, wearable)
		}
		if history != "" {
			t.Errorf("expected %s empty history, got %q", fieldType, history)
		}
		if hash == "" {
			t.Errorf("expected %s hash to be set", fieldType)
		}
		if algoVersion != 1 {
			t.Errorf("expected %s algo_version=1, got %d", fieldType, algoVersion)
		}
		if extra != "" {
			t.Errorf("expected %s empty extra, got %q", fieldType, extra)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("created item field rows failed: %v", err)
	}

	for _, fieldType := range []string{"password", "username", "url"} {
		if !seen[fieldType] {
			t.Errorf("expected created %s field", fieldType)
		}
	}
}

func TestVault_TrashEntry(t *testing.T) {
	tmpDir := copyTestVault(t)
	defer os.RemoveAll(tmpDir)

	vault, err := NewVault(tmpDir, logrus.ErrorLevel)
	if err != nil {
		t.Fatalf("vault initialization failed: %v", err)
	}
	defer vault.Close()

	credentials := &VaultCredentials{Password: testPassword}
	if err := vault.Open(credentials); err != nil {
		t.Skipf("skipping test: could not open vault (environmental issue): %v", err)
	}

	// Get existing entry
	card, err := vault.GetEntry("password", []string{"Whatever"}, true)
	if err != nil {
		t.Fatalf("GetEntry failed: %v", err)
	}

	if card.IsTrashed() {
		t.Error("entry should not be trashed initially")
	}

	// Trash it
	if err := vault.TrashEntry(card.UUID); err != nil {
		t.Fatalf("TrashEntry failed: %v", err)
	}

	// Verify it's trashed
	cards, err := vault.GetEntries("password", []string{"Whatever"})
	if err != nil {
		t.Fatalf("GetEntries failed: %v", err)
	}

	if len(cards) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cards))
	}

	if !cards[0].IsTrashed() {
		t.Error("entry should be trashed")
	}
}

func TestVault_RestoreEntry(t *testing.T) {
	tmpDir := copyTestVault(t)
	defer os.RemoveAll(tmpDir)

	vault, err := NewVault(tmpDir, logrus.ErrorLevel)
	if err != nil {
		t.Fatalf("vault initialization failed: %v", err)
	}
	defer vault.Close()

	credentials := &VaultCredentials{Password: testPassword}
	if err := vault.Open(credentials); err != nil {
		t.Skipf("skipping test: could not open vault (environmental issue): %v", err)
	}

	// Get and trash an entry
	card, _ := vault.GetEntry("password", []string{"Whatever"}, true)
	vault.TrashEntry(card.UUID)

	// Restore it
	if err := vault.RestoreEntry(card.UUID); err != nil {
		t.Fatalf("RestoreEntry failed: %v", err)
	}

	// Verify it's not trashed
	cards, _ := vault.GetEntries("password", []string{"Whatever"})
	if cards[0].IsTrashed() {
		t.Error("entry should not be trashed after restore")
	}
}

func TestVault_DeleteEntry(t *testing.T) {
	tmpDir := copyTestVault(t)
	defer os.RemoveAll(tmpDir)

	vault, err := NewVault(tmpDir, logrus.ErrorLevel)
	if err != nil {
		t.Fatalf("vault initialization failed: %v", err)
	}
	defer vault.Close()

	credentials := &VaultCredentials{Password: testPassword}
	if err := vault.Open(credentials); err != nil {
		t.Skipf("skipping test: could not open vault (environmental issue): %v", err)
	}

	// Create a new entry to delete
	entry := &EntryData{
		Title:    "ToDelete",
		Username: "delete@example.com",
		Password: "deletepassword",
		Category: "Login",
	}
	uuid, _ := vault.CreateEntry(entry)

	// Trash it first
	vault.TrashEntry(uuid)

	// Now delete permanently
	if err := vault.DeleteEntry(uuid); err != nil {
		t.Fatalf("DeleteEntry failed: %v", err)
	}

	// Verify it's gone
	cards, _ := vault.GetEntries("password", []string{"ToDelete"})
	if len(cards) != 0 {
		t.Error("entry should be deleted")
	}
}

func TestVault_UpdateEntry(t *testing.T) {
	tmpDir := copyTestVault(t)
	defer os.RemoveAll(tmpDir)

	vault, err := NewVault(tmpDir, logrus.ErrorLevel)
	if err != nil {
		t.Fatalf("vault initialization failed: %v", err)
	}
	defer vault.Close()

	credentials := &VaultCredentials{Password: testPassword}
	if err := vault.Open(credentials); err != nil {
		t.Skipf("skipping test: could not open vault (environmental issue): %v", err)
	}

	// Get existing entry
	card, _ := vault.GetEntry("password", []string{"Whatever"}, true)
	originalUUID := card.UUID

	updates := &EntryData{
		Title:    "Updated Title",
		Password: "newpassword123",
	}

	if err := vault.UpdateEntry(originalUUID, updates); err != nil {
		t.Fatalf("UpdateEntry failed: %v", err)
	}

	// Verify changes
	cards, _ := vault.GetEntries("password", []string{"Updated Title"})
	if len(cards) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cards))
	}

	if cards[0].Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %q", cards[0].Title)
	}

	decrypted, _ := cards[0].Decrypt()
	if decrypted != "newpassword123" {
		t.Errorf("expected password 'newpassword123', got %q", decrypted)
	}
}
