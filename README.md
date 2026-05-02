# Enpass CLI

A command-line reader for local Enpass vaults, designed for Codex and other
agentic AI runtimes that need controlled access to credentials already stored in
an Enpass vault.

This project is a fork of `github.com/hazcod/enpass-cli`. It is not affiliated
with, endorsed by, or supported by Enpass.

## Scope

Primary goal:

- Open a local Enpass vault.
- Search entries by title or login.
- Return one matching credential in a script-friendly format.
- Support deterministic JSON output for agents.

Non-goals:

- Do not use this as a vault sync client.
- Do not treat this as an official Enpass integration.
- Avoid write operations for automation unless you have tested on a disposable
  vault first.

## Install

With Go:

```shell
go install github.com/leolionart/enpass-cli/cmd/enpass-cli@latest
```

Update to the newest public version:

```shell
go install github.com/leolionart/enpass-cli/cmd/enpass-cli@latest
```

Build from source:

```shell
git clone https://github.com/leolionart/enpass-cli.git
cd enpass-cli
make build
```

The binary is written to:

```shell
./enpass-cli
```

## Vault Path

Pass the vault path explicitly:

```shell
./enpass-cli -vault="/path/to/vault" search github
```

Or set it once for the process:

```shell
export ENPASS_VAULT="/path/to/vault"
```

The path should point to the Enpass vault directory that contains:

- `vault.enpassdb`
- `vault.json`

## Password Input

Interactive prompt:

```shell
./enpass-cli -vault="$ENPASS_VAULT" get github
```

Non-interactive automation:

```shell
ENPASS_MASTER_PASSWORD="your-master-password" \
  ./enpass-cli -vault="$ENPASS_VAULT" -nonInteractive get github
```

`MASTERPW` is still supported for compatibility, but
`ENPASS_MASTER_PASSWORD` is preferred.

Do not pass the vault password as a command argument. Command arguments are
usually visible to local process inspection tools.

## Agent-Friendly Commands

Search matching entries without exposing passwords:

```shell
./enpass-cli -vault="$ENPASS_VAULT" search github
```

Output:

```json
[{"uuid":"...","title":"GitHub","login":"user@example.com","category":"login","label":"password","type":"password","trashed":false}]
```

Get the password for one unique match:

```shell
./enpass-cli -vault="$ENPASS_VAULT" get github
```

Get another field:

```shell
./enpass-cli -vault="$ENPASS_VAULT" -field login get github
./enpass-cli -vault="$ENPASS_VAULT" -field title get github
./enpass-cli -vault="$ENPASS_VAULT" -field uuid get github
```

Get a JSON object containing metadata and password:

```shell
./enpass-cli -vault="$ENPASS_VAULT" -json get github
```

Output:

```json
{"uuid":"...","title":"GitHub","login":"user@example.com","password":"...","category":"login","label":"password","type":"password","trashed":false}
```

Use AND matching when multiple filters must all match:

```shell
./enpass-cli -vault="$ENPASS_VAULT" -and get github user@example.com
```

## Legacy Commands

The original CLI commands are still present:

| Command | Description |
| --- | --- |
| `list FILTER` | List vault entries matching `FILTER` without passwords |
| `show FILTER` | List vault entries matching `FILTER` with passwords |
| `copy FILTER` | Copy the password of one matching entry to the clipboard |
| `pass FILTER` | Print the password of one matching entry to stdout |
| `ui` | Interactive terminal UI |
| `create` | Create a new entry |
| `edit FILTER` | Edit an existing entry |
| `trash FILTER` | Move an entry to trash |
| `restore FILTER` | Restore an entry from trash |
| `delete FILTER` | Permanently delete an entry |
| `dryrun` | Open the vault without reading entries |
| `version` | Print the version |
| `help` | Print help |

For agentic use, prefer `search` and `get`.

## Security Notes

- Prefer read-only usage from agents.
- Use `search` first so the agent can disambiguate without seeing passwords.
- Use `get` only for one exact credential.
- Store `ENPASS_MASTER_PASSWORD` in a secure runtime secret manager such as
  macOS Keychain, not in source control.
- Never commit vault files, exports, passwords, or screenshots containing
  secrets.
- Test against a disposable Enpass vault before pointing automation at a real
  vault.

## Development

Run tests:

```shell
go test ./...
```

Build:

```shell
make build
```

Smoke test with the bundled fixture vault:

```shell
ENPASS_MASTER_PASSWORD="absolutely-No-clue" \
  ./enpass-cli -vault=./test -nonInteractive search Whatever

ENPASS_MASTER_PASSWORD="absolutely-No-clue" \
  ./enpass-cli -vault=./test -nonInteractive get Whatever
```

## License

MIT. See [LICENSE](LICENSE).
