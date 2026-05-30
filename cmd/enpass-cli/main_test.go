package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestDiscoverMacVaultPath(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "Users", "me")
	containerVault := filepath.Join(home, "Library/Containers/in.sinew.Enpass-Desktop/Data/Documents/Vaults/primary")
	documentsVault := filepath.Join(home, "Documents/Enpass/Vaults/primary")

	tests := []struct {
		name       string
		goos       string
		homeErr    error
		existingDB map[string]bool
		want       string
	}{
		{
			name: "ignores non macos",
			goos: "linux",
		},
		{
			name:    "ignores home dir errors",
			goos:    "darwin",
			homeErr: errors.New("no home"),
		},
		{
			name: "prefers container vault",
			goos: "darwin",
			existingDB: map[string]bool{
				filepath.Join(containerVault, "vault.enpassdb"): true,
				filepath.Join(documentsVault, "vault.enpassdb"): true,
			},
			want: containerVault,
		},
		{
			name: "falls back to documents vault",
			goos: "darwin",
			existingDB: map[string]bool{
				filepath.Join(documentsVault, "vault.enpassdb"): true,
			},
			want: documentsVault,
		},
		{
			name:       "returns empty when vault database is absent",
			goos:       "darwin",
			existingDB: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := discoverMacVaultPath(tt.goos, func() (string, error) {
				if tt.homeErr != nil {
					return "", tt.homeErr
				}
				return home, nil
			}, func(path string) bool {
				return tt.existingDB[path]
			})

			if got != tt.want {
				t.Fatalf("discoverMacVaultPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetPasswordFromCommand(t *testing.T) {
	t.Run("empty command returns empty password", func(t *testing.T) {
		got, err := getPasswordFromCommand("", func(string, ...string) ([]byte, error) {
			t.Fatal("exec should not be called")
			return nil, nil
		})
		if err != nil {
			t.Fatalf("getPasswordFromCommand() error = %v", err)
		}
		if got != "" {
			t.Fatalf("getPasswordFromCommand() = %q, want empty string", got)
		}
	})

	t.Run("runs through shell and trims output", func(t *testing.T) {
		got, err := getPasswordFromCommand("secret-tool lookup enpass primary", func(name string, arg ...string) ([]byte, error) {
			if name != "sh" {
				t.Fatalf("exec name = %q, want sh", name)
			}
			if len(arg) != 2 || arg[0] != "-c" || arg[1] != "secret-tool lookup enpass primary" {
				t.Fatalf("exec args = %#v, want sh -c command", arg)
			}
			return []byte("  vault-pass\n"), nil
		})
		if err != nil {
			t.Fatalf("getPasswordFromCommand() error = %v", err)
		}
		if got != "vault-pass" {
			t.Fatalf("getPasswordFromCommand() = %q, want vault-pass", got)
		}
	})

	t.Run("returns command errors", func(t *testing.T) {
		wantErr := errors.New("exit 1")
		_, err := getPasswordFromCommand("false", func(string, ...string) ([]byte, error) {
			return nil, wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("getPasswordFromCommand() error = %v, want %v", err, wantErr)
		}
	})
}
