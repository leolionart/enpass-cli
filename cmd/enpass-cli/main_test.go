package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestDiscoverMacVaultPath(t *testing.T) {
	// Test case: not on macOS (darwin)
	t.Run("non-darwin", func(t *testing.T) {
		res := discoverMacVaultPath("linux", func() (string, error) {
			return "/home/user", nil
		}, func(p string) bool {
			return true
		})
		if res != "" {
			t.Errorf("expected empty vault path on non-darwin OS, got %q", res)
		}
	})

	// Test case: error getting user home dir
	t.Run("home dir error", func(t *testing.T) {
		res := discoverMacVaultPath("darwin", func() (string, error) {
			return "", errors.New("failed to get home dir")
		}, func(p string) bool {
			return true
		})
		if res != "" {
			t.Errorf("expected empty vault path when home dir error occurs, got %q", res)
		}
	})

	// Test case: primary container path exists
	t.Run("container path exists", func(t *testing.T) {
		expectedContainerPath := filepath.Join("/Users/test", "Library/Containers/in.sinew.Enpass-Desktop/Data/Documents/Vaults/primary")
		res := discoverMacVaultPath("darwin", func() (string, error) {
			return "/Users/test", nil
		}, func(p string) bool {
			return p == filepath.Join(expectedContainerPath, "vault.enpassdb")
		})
		if res != expectedContainerPath {
			t.Errorf("expected %q, got %q", expectedContainerPath, res)
		}
	})

	// Test case: primary Documents path exists
	t.Run("documents path exists", func(t *testing.T) {
		expectedDocsPath := filepath.Join("/Users/test", "Documents/Enpass/Vaults/primary")
		res := discoverMacVaultPath("darwin", func() (string, error) {
			return "/Users/test", nil
		}, func(p string) bool {
			return p == filepath.Join(expectedDocsPath, "vault.enpassdb")
		})
		if res != expectedDocsPath {
			t.Errorf("expected %q, got %q", expectedDocsPath, res)
		}
	})

	// Test case: neither exists
	t.Run("neither exists", func(t *testing.T) {
		res := discoverMacVaultPath("darwin", func() (string, error) {
			return "/Users/test", nil
		}, func(p string) bool {
			return false
		})
		if res != "" {
			t.Errorf("expected empty path when neither exists, got %q", res)
		}
	})
}

func TestGetPasswordFromCommand(t *testing.T) {
	// Test case: empty command string
	t.Run("empty command", func(t *testing.T) {
		res, err := getPasswordFromCommand("", func(name string, arg ...string) ([]byte, error) {
			return []byte("secret"), nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "" {
			t.Errorf("expected empty password, got %q", res)
		}
	})

	// Test case: command executes successfully and whitespace is trimmed
	t.Run("success command execution", func(t *testing.T) {
		res, err := getPasswordFromCommand("echo ' mysecret '", func(name string, arg ...string) ([]byte, error) {
			if name != "sh" || len(arg) != 2 || arg[0] != "-c" || arg[1] != "echo ' mysecret '" {
				return nil, errors.New("unexpected command invocation")
			}
			return []byte(" mysecret \n"), nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != "mysecret" {
			t.Errorf("expected 'mysecret', got %q", res)
		}
	})

	// Test case: command execution fails
	t.Run("failed command execution", func(t *testing.T) {
		_, err := getPasswordFromCommand("false", func(name string, arg ...string) ([]byte, error) {
			return nil, errors.New("command failed")
		})
		if err == nil {
			t.Error("expected error on command failure, got nil")
		}
	})
}
