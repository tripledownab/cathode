# Building cathode locally

Requirements: **Go 1.22+** and the **`claude` CLI** (logged in with your Pro/Max
account: `claude login`).

```bash
cd cathode
go mod tidy        # fetches deps + writes go.sum (needs network the first time)
make build         # -> ./cathode     (or: go build -o cathode .)
./cathode
```

Other targets: `make run`, `make test`, `make clean`.

Flags: `-mode ask|plan|build|bypass`, `-spinner bar|shade|block|arrow|scan`,
`-mcp <path-to-.mcp.json>`, `-model <name>`.

Note: the shipped `go.mod` has no `go.sum` on purpose — `go mod tidy` generates
it against the canonical module proxy on your machine. Confirm `/status` in
`claude` shows the subscription route before relying on Max billing.
