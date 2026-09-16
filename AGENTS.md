# web_roguelike

## Commands

- Use Go `1.23.3`. Template generation requires templ `v0.3.906`; install it with `go install github.com/a-h/templ/cmd/templ@v0.3.906` if needed.
- Run: `go run server.go` (defaults to `http://localhost:9779`).
- Run custom endpoint: `go run server.go -address=http://localhost -port=8080`.
- Run from repository root because static files use relative path `./static`.
- Build: `go build .`.
- All tests: `go test ./...`.
- Focused package: `go test ./game`.
- Exact test: `go test ./path/to/package -run '^TestName$'`.
- Vet: `go vet ./...`.
- Format recursively: `go fmt ./...` (not `gofmt -w .`).
- No `*_test.go` files currently exist.

## Templates

- Template sources are `templ/*.templ`.
- Generated siblings are checked in as `templ/*_templ.go`; generated files contain `DO NOT EDIT`.
- After editing a template, run `templ generate` and include generated-file changes.
- Source and generated tree is currently stale: `templ/map.templ` uses `border-width: 3px`, while `templ/map_templ.go` uses `5px`.
- Do not edit generated files manually; expect first regeneration to change this stale output.
- `templ/test.templ` is a UI component, not a test suite.

## HTTP and sessions

- `server.go` is the sole executable and wires `/`, `/map`, `/test`, and `/static/`.
- Handlers render templ components and always get/set the session cookie.
- `session` stores a process-local map protected by one mutex.
- New session generation creates player/map and positions player at `StartPos`.
- Restart loses sessions. No cleanup, expiry enforcement, or multi-process sharing exists, despite the cookie's 24-hour expiry.

## API quirk

- Current generation methods are `Generate_player`, `Generate_room`, and `Generate_map`; do not call invented PascalCase variants.
