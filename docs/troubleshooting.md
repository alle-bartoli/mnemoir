# Troubleshooting

## Binary not found after `task install`

Add `$(go env GOPATH)/bin` to `PATH` on macOS/Linux. On Windows, add the Go `bin` directory from `go env GOPATH` to the user `PATH`, then restart the terminal.

## Windows reports `jq is required` or uses `/.mnemoir`

Use Git Bash rather than WSL Bash. If Git is installed outside the default location, set:

```powershell
$env:MNEMOIR_BASH = "C:/path/to/Git/bin/bash.exe"
task setup
```

## First local model startup is slow

The default local embedding model is about 90 MB. Run `task prewarm` before connecting an MCP client. 
The command is skipped for non-local embedding providers.

## Claude Desktop reports permission denied on macOS

Remove quarantine or ad-hoc sign the binary:

```bash
xattr -dr com.apple.quarantine "$(go env GOPATH)/bin/mnemoir"
codesign --force --deep --sign - "$(go env GOPATH)/bin/mnemoir"
```

## Logs

Logs are written to `~/.mnemoir/mnemoir.log`. The fallback output is stderr.
