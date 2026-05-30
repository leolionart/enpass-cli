# Enpass CLI

Command-line tool for reading a local Enpass vault.

On macOS, the normal setup is simple: install the CLI, run a command, let it
find your Enpass vault, then enter your Enpass master password when asked.

This project is a fork of `github.com/hazcod/enpass-cli`. It is not affiliated
with, endorsed by, or supported by Enpass.

## Quick Start

### 1. Install

If you use Homebrew on macOS:

```shell
GOBIN=/opt/homebrew/bin go install github.com/leolionart/enpass-cli/cmd/enpass-cli@latest
```

If you do not use Homebrew:

```shell
go install github.com/leolionart/enpass-cli/cmd/enpass-cli@latest
```

### 2. Check that it works

```shell
enpass-cli version
```

### 3. Search your Enpass vault

```shell
enpass-cli search github
```

What happens:

1. `enpass-cli` tries to find your default macOS Enpass vault.
2. It asks for your Enpass master password.
3. It prints matching entries as JSON.

### 4. Get one password

```shell
enpass-cli get github
```

Use a more specific search if more than one entry matches:

```shell
enpass-cli get github user@example.com
```

## Default Vault Detection

When `-vault` and `ENPASS_VAULT` are not set, macOS checks these paths:

- `~/Library/Containers/in.sinew.Enpass-Desktop/Data/Documents/Vaults/primary`
- `~/Documents/Enpass/Vaults/primary`

The selected folder must contain `vault.enpassdb`.

<details>
<summary>Use a custom vault path</summary>

If your vault is somewhere else, pass the path directly:

```shell
enpass-cli -vault="/path/to/vault-folder" search github
```

Or set it for the current terminal session:

```shell
export ENPASS_VAULT="/path/to/vault-folder"
enpass-cli search github
```

To keep it permanently on macOS zsh:

```shell
echo 'export ENPASS_VAULT="/path/to/vault-folder"' >> ~/.zshrc
source ~/.zshrc
```

</details>

<details>
<summary>If the install command says Go is missing</summary>

Install Go first:

```shell
brew install go
```

Or download the official installer from:

https://go.dev/dl/

Then run the install command again.

</details>

<details>
<summary>If enpass-cli is not found after install</summary>

If you installed with `GOBIN=/opt/homebrew/bin`, make sure Homebrew's binary
folder is in your shell path:

```shell
echo 'export PATH="/opt/homebrew/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

If you installed with plain `go install`, add Go's default binary folder:

```shell
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

</details>

## Common Commands

```shell
enpass-cli search github
enpass-cli get github
enpass-cli -field login get github
enpass-cli -json get github
enpass-cli -details search 172.16.0
enpass-cli -and get github user@example.com
```

`search` does not print passwords. `get` prints the selected field, which is
`password` by default.

<details>
<summary>Password input options</summary>

Manual use:

```shell
enpass-cli search github
```

The CLI asks for your master password in a hidden terminal prompt.

Automation with macOS Keychain or another password command:

```shell
enpass-cli \
  -password-command="security find-generic-password -w -s 'Enpass Master Password'" \
  -nonInteractive \
  get github
```

Or through an environment variable:

```shell
ENPASS_PASSWORD_COMMAND="security find-generic-password -w -s 'Enpass Master Password'" \
  enpass-cli -nonInteractive get github
```

The command runs through `sh -c`. Its stdout is trimmed and used as the vault
password.

Simple local scripts can also use:

```shell
ENPASS_MASTER_PASSWORD="your-master-password" enpass-cli -nonInteractive get github
```

`MASTERPW` is still supported, but `ENPASS_MASTER_PASSWORD` is preferred.

</details>

<details>
<summary>Categories and infrastructure cleanup</summary>

List categories:

```shell
enpass-cli -json categories
```

Create a custom category:

```shell
enpass-cli -json categories create "Home Lab"
```

Delete a custom category after moving its items elsewhere:

```shell
enpass-cli -json categories delete "Home Lab"
```

Use a category name or UUID when creating or editing an entry:

```shell
enpass-cli -category SSH -title "SSH Web Server" create
```

Preview infrastructure categorization:

```shell
enpass-cli -json -dry-run categorize-infra > infra-dry-run.json
```

Apply the reviewed dry-run result:

```shell
enpass-cli -json -apply -from-dry-run infra-dry-run.json categorize-infra
```

`categorize-infra` backs up the vault directory before live changes.

</details>

<details>
<summary>Sorting and legacy commands</summary>

Sort examples:

```shell
enpass-cli -sort=updated search github
enpass-cli -sort=created search
enpass-cli -sort=used list
enpass-cli -sort=usage list
```

Supported sort keys: `title`, `login`, `created`, `updated`, `used`, `usage`.

Legacy commands are still available:

| Command | Description |
| --- | --- |
| `list FILTER` | List entries without passwords |
| `show FILTER` | List entries with passwords |
| `copy FILTER` | Copy the password to the clipboard |
| `pass FILTER` | Print the password |
| `ui` | Open the terminal UI |
| `create` | Create a vault entry |
| `edit FILTER` | Edit a vault entry |
| `trash FILTER` | Move an entry to trash |
| `restore FILTER` | Restore a trashed entry |
| `delete FILTER` | Permanently delete an entry |
| `dryrun` | Open the vault without reading entries |
| `version` | Print the CLI version |
| `help` | Print help |

</details>

<details>
<summary>Security notes</summary>

- Do not put the actual vault password directly in a command argument.
- Prefer a password manager or macOS Keychain for automation.
- Use `search` before `get` so scripts can identify the right entry without
  exposing passwords.
- Test write operations on a disposable vault before using a real vault.
- Never commit vault files, exports, passwords, or screenshots containing
  secrets.

</details>

<details>
<summary>Development</summary>

Build from source:

```shell
git clone https://github.com/leolionart/enpass-cli.git
cd enpass-cli
make build
```

Run tests:

```shell
go test ./...
```

Smoke test with the bundled fixture vault:

```shell
ENPASS_MASTER_PASSWORD="absolutely-No-clue" ./enpass-cli -vault=./test -nonInteractive search Whatever
ENPASS_MASTER_PASSWORD="absolutely-No-clue" ./enpass-cli -vault=./test -nonInteractive get Whatever
```

</details>

## License

MIT. See [LICENSE](LICENSE).
