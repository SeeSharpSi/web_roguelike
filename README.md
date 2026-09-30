# Shared Roguelike

Run the server from the repository root:

```sh
go run server.go
```

Open `http://localhost:9779`, create a room with a username, then share its room URL or code. Other players join with a username. To reconnect after losing a browser session, enter the same room code and username. The latest connection controls the existing player and replaces its previous session binding. Explicitly leaving abandons the player.

Rooms use one shared map. The host starts and finishes the game. All room and session state is held in memory and is lost when the server restarts. Idle sessions expire after 24 hours and idle rooms after two hours.

Usernames are case-insensitive, with surrounding whitespace ignored, and identify players within one room. Each room holds up to eight players. New players join while the room is in the lobby; existing players can reconnect during play or after the game finishes. When the host leaves, another remaining player becomes the host.

## Routes

- `GET /`: Create or join room forms.
- `POST /rooms`: Create a room.
- `POST /rooms/join` and `POST /rooms/{code}/join`: Join or reconnect.
- `GET /rooms/{code}`: Room page or join form.
- `GET /rooms/{code}/state`: Current room fragment.
- `POST /rooms/{code}/start`: The host starts the game.
- `POST /rooms/{code}/actions`: Move the current player using a `direction` form field.
- `POST /rooms/{code}/finish`: The host finishes the game.
- `POST /rooms/{code}/leave`: Abandon the current player.
- `GET /test`: Template demo.

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
go build -o /dev/null .
```

With the server running, execute `scripts/test-rooms.playwright.js` through the Playwright MCP `browser_run_code_unsafe` tool using its `filename` parameter. The suite uses independent browser contexts to check shared maps, room isolation, movement, reconnects after losing cookies, previous-session revocation, host transfer, and room closure.
