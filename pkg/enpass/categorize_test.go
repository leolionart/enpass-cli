package enpass

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestVault_BuildInfraCategorizationReport(t *testing.T) {
	tmpDir := copyTestVault(t)
	defer os.RemoveAll(tmpDir)

	vault := openTestVault(t, tmpDir)
	defer vault.Close()

	if _, err := vault.CreateEntry(&EntryData{
		Title:    "Home Server",
		Username: "root",
		Password: "secret",
		URL:      "https://172.16.0.14:8006",
		Category: "login",
	}); err != nil {
		t.Fatalf("CreateEntry failed: %v", err)
	}

	report, err := vault.BuildInfraCategorizationReport()
	if err != nil {
		t.Fatalf("BuildInfraCategorizationReport failed: %v", err)
	}

	found := false
	for _, item := range report.Items {
		if item.Title == "Home Server" {
			found = true
			if item.TargetCategory != "Password" {
				t.Fatalf("expected Home Server target Password, got %q", item.TargetCategory)
			}
			if item.CurrentCategory != "login" {
				t.Fatalf("expected current category login, got %q", item.CurrentCategory)
			}
		}
	}
	if !found {
		t.Fatal("expected Home Server in report")
	}
}

func TestVault_ApplyInfraCategorizationReport(t *testing.T) {
	tmpDir := copyTestVault(t)
	defer os.RemoveAll(tmpDir)

	vault := openTestVault(t, tmpDir)
	defer vault.Close()

	entryUUID, err := vault.CreateEntry(&EntryData{
		Title:    "Mikrotik Router",
		Username: "admin",
		Password: "secret",
		URL:      "http://172.16.0.1",
		Category: "login",
	})
	if err != nil {
		t.Fatalf("CreateEntry failed: %v", err)
	}

	report, err := vault.BuildInfraCategorizationReport()
	if err != nil {
		t.Fatalf("BuildInfraCategorizationReport failed: %v", err)
	}

	result, err := vault.ApplyInfraCategorizationReport(report)
	if err != nil {
		t.Fatalf("ApplyInfraCategorizationReport failed: %v", err)
	}
	if len(result.CategoriesCreated) != 0 {
		t.Fatalf("expected no categories to be created for built-in Password, got %d", len(result.CategoriesCreated))
	}

	card, err := vault.GetEntryByUUID(entryUUID)
	if err != nil {
		t.Fatalf("GetEntryByUUID failed: %v", err)
	}
	passwordUUID, err := vault.ResolveCategory("Password")
	if err != nil {
		t.Fatalf("ResolveCategory Password failed: %v", err)
	}
	if card.Category != passwordUUID {
		t.Fatalf("expected Password category UUID %q, got %q", passwordUUID, card.Category)
	}
}

func openTestVault(t *testing.T, path string) *Vault {
	t.Helper()

	vault, err := NewVault(path, logrus.ErrorLevel)
	if err != nil {
		t.Fatalf("vault initialization failed: %v", err)
	}

	credentials := &VaultCredentials{Password: testPassword}
	if err := vault.Open(credentials); err != nil {
		vault.Close()
		t.Skipf("skipping test: could not open vault (environmental issue): %v", err)
	}

	return vault
}
