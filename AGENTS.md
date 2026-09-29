# web_roguelike

## Commands

- `go.mod` declares Go `1.23.3`.
- Run from the repository root: `go run server.go`, default `http://localhost:9779`. Static assets resolve from `./static`.
- `-address` requires a URL scheme: `go run server.go -address=http://localhost -port=8080`.
- Build check: `go build -o /dev/null .`. `go build .` overwrites the tracked `web_roguelike` binary.
- Verify with `go test ./...` and `go vet ./...`. Focus game logic with `go test ./game`, rendering with `go test ./templ`.
- Single map regression: `go test ./game -run '^TestGenerateMapWallQuotasAndConnectivity$'`.

## Templates

- Edit `templ/*.templ`; checked-in `templ/*_templ.go` files are generated. Do not edit generated files manually.
- Match the generator to templ runtime `v0.3.906`: `go install github.com/a-h/templ/cmd/templ@v0.3.906`.
- After template edits, run `templ generate` from the repository root before build/test and include generated-file changes.
- The map output is stale: `templ/map.templ` uses `border-width: 3px`, but `templ/map_templ.go` uses `5px`. Expect regeneration to reconcile this.

## Execution and sessions

- `server.go` wires handlers in `handlers/`; they render `templ/` components using player/map state from `session/` and generation logic in `game/`.
- `/`, `/map`, and `/test` all create/reuse a `session_id` session and set its cookie. New sessions generate a player/map and place the player at `Map.StartPos`.
- Sessions are process-local and disappear on restart. The server does not expire or clean them up despite the cookie's 24-hour expiry.
- The session-manager mutex protects store access, not later mutation of returned `*Session` values.

## Game quirks

- Generation methods are `Generate_player`, `Generate_room`, and `Generate_map`. Room/map generation accepts an optional `*rand.Rand` from `math/rand/v2`; use `rand.New(rand.NewPCG(seed1, seed2))` for deterministic tests.
- Shared walls are stored as values in both adjacent rooms; update both sides together.
- Map coordinates have `(0,0)` at bottom-left; `templ/map.templ` renders Y in descending order.
