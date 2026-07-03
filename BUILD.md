# Building cathode locally

Requirements: **Go 1.22+** and the **`claude` CLI** (logged in with your Pro/Max
account: `claude login`).

```bash
cd cathode
go mod download    # fetches deps (needs network the first time)
make build         # -> ./cathode     (or: go build -o cathode .)
./cathode
```

Other targets: `make run`, `make test`, `make tidy`, `make install` (to
`~/.local/bin`, override with `PREFIX=`), `make uninstall`, `make reinstall`,
`make watch` (rebuild+reinstall on save, needs `entr`), `make clean`.

Flags: `-mode ask|plan|build|bypass`, `-spinner bar|shade|block|arrow|scan`,
`-mcp <path-to-.mcp.json>`, `-model <name>`, `-resume <session-id>`,
`-ctx 200k|500k|1m|<n>`, `-debug <logfile>`.

Note: confirm `/status` in `claude` shows the subscription route before
relying on Max billing.
