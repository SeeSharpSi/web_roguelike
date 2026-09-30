# web_roguelike

## Commands

- `go.mod` declares Go `1.23.3`.
- Run from the repository root: `go run server.go`, default `http://localhost:9779`. Static assets resolve from `./static`.
- `-address` requires a URL scheme: `go run server.go -address=http://localhost -port=8080`.
- Build check: `go build -o /dev/null .`. `go build .` overwrites the tracked `web_roguelike` binary.
- Verify with `go test ./...` and `go vet ./...`. Focus game logic with `go test ./game`, rendering with `go test ./templ`.
- Verify multiplayer synchronization with `go test -race ./match ./session ./handlers`.
- Single map regression: `go test ./game -run '^TestGenerateMapWallQuotasAndConnectivity$'`.

## Templates

- Edit `templ/*.templ`; checked-in `templ/*_templ.go` files are generated. Do not edit generated files manually.
- Match the generator to templ runtime `v0.3.906`: `go install github.com/a-h/templ/cmd/templ@v0.3.906`.
- After template edits, run `templ generate` from the repository root before build/test and include generated-file changes.
- Include regenerated map output when changing its source. Shared walls render once at 3px thickness: normal walls `#E8DEC3`, doors `#4AA6C8`, and destructible walls `#D85B52`.

## Execution and sessions

- `server.go` wires handlers in `handlers/`, an identity-only `session.Manager`, and a shared-game `match.Registry`. Templates render independent match snapshots; `game/` owns generation and movement rules.
- Room creation and joining use a username. Normalized usernames identify players within one match; joining an existing username reconnects to that player's state and replaces the previous session binding.
- A match owns one map and separate players. Game actions and snapshots authorize the requesting session and synchronize access through a per-match mutex.
- Session access returns copies. Match snapshots must deep-copy map entries, room pointers, and item slices before releasing the match lock.
- Sessions and matches are process-local and disappear on restart. Background cleanup removes sessions idle for 24 hours and matches idle for two hours; expiring a session does not delete its player.
- New players join only in the lobby. Existing players can reconnect after the game starts or finishes. Explicitly leaving abandons the player; the host transfers to a remaining player.

## Game quirks

- Generation methods are `Generate_player`, `Generate_room`, and `Generate_map`. Room/map generation accepts an optional `*rand.Rand` from `math/rand/v2`; use `rand.New(rand.NewPCG(seed1, seed2))` for deterministic tests.
- Shared walls are stored as values in both adjacent rooms; update both sides together.
- Map coordinates have `(0,0)` at bottom-left; `templ/map.templ` renders Y in descending order.
- The tactical chart labels columns alphabetically and screen rows from top to bottom. A displayed row number is `Map.Length - Position.Y`.
- The frontend follows the read-only `web_roguelike` Paper mockup. Keep its visual palette and panel proportions while binding HUD values to real match snapshots.
