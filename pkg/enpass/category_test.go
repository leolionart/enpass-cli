package enpass

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestVault_ListAndResolveCategories(t *testing.T) {
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

	_, err = vault.db.Exec(`
		INSERT INTO category (uuid, title, icon, updated_at, deleted)
		VALUES (?, ?, ?, ?, 0)
	`, "54507015-f8ec-4e40-86d4-1b77103a5656", "SSH", "1010", int64(1778645565))
	if err != nil {
		t.Fatalf("could not seed category: %v", err)
	}

	categories, err := vault.ListCategories()
	if err != nil {
		t.Fatalf("ListCategories failed: %v", err)
	}
	if len(categories) < 2 {
		t.Fatalf("expected builtin and custom categories, got %d", len(categories))
	}

	resolved, err := vault.ResolveCategory("SSH")
	if err != nil {
		t.Fatalf("ResolveCategory by title failed: %v", err)
	}
	if resolved != "54507015-f8ec-4e40-86d4-1b77103a5656" {
		t.Fatalf("expected SSH UUID, got %q", resolved)
	}

	resolved, err = vault.ResolveCategory("login")
	if err != nil {
		t.Fatalf("ResolveCategory builtin failed: %v", err)
	}
	if resolved != "login" {
		t.Fatalf("expected builtin login, got %q", resolved)
	}

	resolved, err = vault.ResolveCategory("Password")
	if err != nil {
		t.Fatalf("ResolveCategory built-in password failed: %v", err)
	}
	if resolved != "password" {
		t.Fatalf("expected builtin password, got %q", resolved)
	}
}

func TestVault_CreateCategoryIsIdempotent(t *testing.T) {
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

	first, err := vault.CreateCategory("Home Lab", "")
	if err != nil {
		t.Fatalf("CreateCategory failed: %v", err)
	}
	if !first.Created {
		t.Fatal("expected first category create to create a row")
	}

	second, err := vault.CreateCategory("home lab", "")
	if err != nil {
		t.Fatalf("CreateCategory second call failed: %v", err)
	}
	if second.Created {
		t.Fatal("expected second category create to reuse existing row")
	}
	if first.Category.UUID != second.Category.UUID {
		t.Fatalf("expected same category UUID, got %q and %q", first.Category.UUID, second.Category.UUID)
	}
}

func TestVault_DeleteCustomCategory(t *testing.T) {
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

	created, err := vault.CreateCategory("SSH", "1010")
	if err != nil {
		t.Fatalf("CreateCategory failed: %v", err)
	}

	deleted, err := vault.DeleteCategory("SSH")
	if err != nil {
		t.Fatalf("DeleteCategory failed: %v", err)
	}
	if deleted.UUID != created.Category.UUID {
		t.Fatalf("expected deleted UUID %q, got %q", created.Category.UUID, deleted.UUID)
	}

	if _, err := vault.ResolveCategory("SSH"); err == nil {
		t.Fatal("expected deleted category to stop resolving")
	}
	if _, err := vault.DeleteCategory("password"); err == nil {
		t.Fatal("expected built-in category delete to fail")
	}
}
