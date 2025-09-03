# AGENTS.md - Development Guidelines for web_roguelike

## Build/Lint/Test Commands

### Building and Running
- **Build**: `go build .` - Compiles the Go application
- **Run**: `go run server.go` - Runs the server directly (default port 9779)
- **Run with custom port**: `go run server.go -port=8080 -address=http://localhost`

### Testing
- **Run all tests**: `go test ./...` - Executes all tests in the project
- **Run specific package tests**: `go test ./game` - Tests only the game package
- **Run single test**: `go test -run TestFunctionName ./package`

### Linting and Formatting
- **Format code**: `gofmt -w .` - Formats all Go files in place
- **Check formatting**: `gofmt -d .` - Shows formatting differences without changing files
- **Vet code**: `go vet ./...` - Runs Go's static analysis tool
- **Generate templ files**: `templ generate` - Generates Go code from .templ files

### Development Workflow
1. `templ generate` - Generate templ files after modifying .templ files
2. `go build .` - Build the application
3. `go run server.go` - Start the development server

## Code Style Guidelines

### Naming Conventions
- **Packages**: lowercase, single word (e.g., `game`, `handlers`, `session`)
- **Exported types/functions**: PascalCase (e.g., `Player`, `GenerateMap`, `NewManager`)
- **Unexported types/functions**: camelCase (e.g., `generateRoom`, `getSession`)
- **Variables**: camelCase (e.g., `player`, `sessionID`, `wallTypes`)
- **Constants**: PascalCase (e.g., `MaxHealth`)

### Imports
```go
import (
    "context"
    "net/http"

    "github.com/a-h/templ"
    "seesharpsi/web_roguelike/game"
    "seesharpsi/web_roguelike/session"
)
```
- Group standard library imports first
- Third-party imports second
- Local imports last
- Use blank lines between groups

### Structs and Types
- Use meaningful names that describe purpose
- Group related fields together
- Add comments for complex structs
- Example:
```go
// Player represents a game character with stats and position
type Player struct {
    Alive    bool
    Health   int
    Strength float64 // Strength is a multiplier
    Stamina  int
    Position Pos
}
```

### Functions
- Exported functions should have clear, descriptive names
- Use receiver names that match the type (e.g., `func (p *Player)`)
- Keep functions focused on single responsibility
- Example:
```go
func (p *Player) GeneratePlayer() {
    // Implementation
}
```

### Error Handling
- Use standard Go error patterns
- Check errors immediately after operations
- Return errors from functions that can fail
- Example:
```go
func (m *Manager) GetSession(id string) (*Session, error) {
    session, ok := m.sessions[id]
    if !ok {
        return nil, errors.New("session not found")
    }
    return session, nil
}
```

### Comments
- Add comments for exported functions and types
- Use complete sentences starting with the name being described
- Example:
```go
// GenerateMap creates a series of connected rooms using a randomized DFS algorithm
func (m *Map) GenerateMap() {
    // Implementation
}
```

### Code Organization
- Keep related functionality in the same package
- Use meaningful package names that reflect functionality
- Separate concerns (handlers, game logic, session management)
- Follow Go's idiomatic file structure

### Templ Files
- Use PascalCase for component names (e.g., `Index()`, `Map()`)
- Keep templ logic simple, delegate complex logic to Go functions
- Use meaningful variable names in templ expressions
- Example:
```go
templ Map(gameMap game.Map, player game.Player) {
    // Template implementation
}
```

### HTTP Handlers
- Use descriptive handler names (e.g., `Index`, `Map`, `Test`)
- Extract common session logic into helper functions
- Keep handlers focused on HTTP concerns
- Example:
```go
func (h *Handler) Map(w http.ResponseWriter, r *http.Request) {
    sess, cookie := h.Manager.GetOrCreateSession(r)
    http.SetCookie(w, &cookie)
    // Handler logic
}
```

### Constants and Magic Numbers
- Use named constants instead of magic numbers
- Group related constants together
- Example:
```go
const (
    DefaultHealth = 100
    MaxStamina    = 50
)
```

### Logging
- Use `log.Printf()` for simple logging
- Include context in log messages
- Use appropriate log levels (info, error)
- Example:
```go
log.Printf("running server on %s\n", root_ip.Host)
log.Printf("error starting server: %s\n", err)
```

### Security Best Practices
- Use `crypto/rand` for generating secure random values
- Set appropriate HTTP headers (HttpOnly cookies)
- Validate user input
- Use secure session management

### Performance Considerations
- Use pointers for large structs to avoid copying
- Pre-allocate slices when size is known
- Use efficient algorithms for game logic
- Consider memory usage in long-running server processes