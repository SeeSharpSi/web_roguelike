package match_test

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"seesharpsi/web_roguelike/game"
	"seesharpsi/web_roguelike/match"
)

func TestRoomsAreIndependentAndMembersShareRoomState(t *testing.T) {
	registry := match.NewRegistry()
	first, err := registry.Create("session-one", "Alex")
	if err != nil {
		t.Fatal(err)
	}
	secondRoom, err := registry.Create("session-two", "Alex")
	if err != nil {
		t.Fatal(err)
	}
	if first.Viewer.ID == secondRoom.Viewer.ID {
		t.Fatal("same username in separate rooms received same player ID")
	}

	member, err := registry.Join(strings.ToLower("  "+first.Code+"  "), "session-three", "Blair")
	if err != nil {
		t.Fatal(err)
	}
	if member.Viewer.ID == first.Viewer.ID {
		t.Fatal("different room members received same player ID")
	}
	if !reflect.DeepEqual(first.World, member.World) {
		t.Fatal("members of one room observed different world state")
	}
	if member.Viewer.ID == secondRoom.Viewer.ID {
		t.Fatal("member in one room reused player ID from another room")
	}
	if _, err := registry.Start(first.Code, "session-one"); err != nil {
		t.Fatal(err)
	}
	otherRoom, err := registry.Snapshot(secondRoom.Code, "session-two")
	if err != nil {
		t.Fatal(err)
	}
	if otherRoom.Status != match.Lobby {
		t.Fatalf("starting first room changed second room status to %v", otherRoom.Status)
	}
	if got := match.NormalizeCode("  " + strings.ToLower(first.Code) + "  "); got != first.Code {
		t.Fatalf("NormalizeCode(%q) = %q, want %q", first.Code, got, first.Code)
	}
	if _, err := registry.Snapshot("  "+strings.ToLower(first.Code)+"  ", "session-three"); err != nil {
		t.Fatalf("normalized room code did not resolve: %v", err)
	}
}

func TestReconnectRetainsPlayerAndWorldAndRevokesOldSession(t *testing.T) {
	registry := match.NewRegistry()
	progressed, oldSession := createStartedMatchWithMove(t, registry, "reconnect")
	if progressed.Viewer.Username != "Walker" {
		t.Fatalf("first display name = %q, want Walker", progressed.Viewer.Username)
	}

	newSession := oldSession + "-replacement"
	reconnected, err := registry.Join(progressed.Code, newSession, "  wAlKeR  ")
	if err != nil {
		t.Fatal(err)
	}
	if reconnected.Viewer.ID != progressed.Viewer.ID {
		t.Fatalf("reconnect player ID = %q, want %q", reconnected.Viewer.ID, progressed.Viewer.ID)
	}
	if reconnected.Viewer.Username != "Walker" {
		t.Fatalf("reconnect changed first display name to %q", reconnected.Viewer.Username)
	}
	if reconnected.Viewer.Player != progressed.Viewer.Player {
		t.Fatalf("reconnect changed player state: got %+v, want %+v", reconnected.Viewer.Player, progressed.Viewer.Player)
	}
	if !reflect.DeepEqual(reconnected.World, progressed.World) {
		t.Fatal("reconnect changed shared world state")
	}
	if len(reconnected.Players) != len(progressed.Players) {
		t.Fatalf("reconnect changed player count: got %d, want %d", len(reconnected.Players), len(progressed.Players))
	}

	repeated, err := registry.Join(progressed.Code, newSession, "WALKER")
	if err != nil {
		t.Fatalf("repeat join failed: %v", err)
	}
	if repeated.Viewer.ID != progressed.Viewer.ID || len(repeated.Players) != len(progressed.Players) {
		t.Fatalf("repeat join was not idempotent: viewer=%q players=%d", repeated.Viewer.ID, len(repeated.Players))
	}
	if _, err := registry.Snapshot(progressed.Code, oldSession); !errors.Is(err, match.ErrNotMember) {
		t.Fatalf("old session Snapshot error = %v, want ErrNotMember", err)
	}
	if _, err := registry.Move(progressed.Code, oldSession, "north"); !errors.Is(err, match.ErrNotMember) {
		t.Fatalf("old session Move error = %v, want ErrNotMember", err)
	}
}

func TestConcurrentReconnectsBindOneSessionAndOnePlayer(t *testing.T) {
	registry := match.NewRegistry()
	room, err := registry.Create("host-session", "Host")
	if err != nil {
		t.Fatal(err)
	}

	const attempts = 16
	start := make(chan struct{})
	results := make(chan error, attempts)
	var workers sync.WaitGroup
	for i := 0; i < attempts; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			name := "  aDa  "
			if i%2 != 0 {
				name = "ADA"
			}
			_, err := registry.Join(room.Code, fmt.Sprintf("candidate-%02d", i), name)
			results <- err
		}(i)
	}
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent reconnect failed: %v", err)
		}
	}

	final, err := registry.Snapshot(room.Code, "host-session")
	if err != nil {
		t.Fatal(err)
	}
	if len(final.Players) != 2 {
		t.Fatalf("concurrent same-name joins created %d players, want host plus one reconnecting player", len(final.Players))
	}
	var adaID string
	for _, player := range final.Players {
		if strings.EqualFold(strings.TrimSpace(player.Username), "ada") {
			if adaID != "" {
				t.Fatal("concurrent same-name joins created duplicate normalized usernames")
			}
			adaID = player.ID
		}
	}
	if adaID == "" {
		t.Fatal("reconnecting player missing from room")
	}

	validBindings := 0
	for i := 0; i < attempts; i++ {
		_, err := registry.Snapshot(room.Code, fmt.Sprintf("candidate-%02d", i))
		switch {
		case err == nil:
			validBindings++
		case errors.Is(err, match.ErrNotMember):
		default:
			t.Fatalf("candidate Snapshot error = %v, want success or ErrNotMember", err)
		}
	}
	if validBindings != 1 {
		t.Fatalf("concurrent reconnect left %d valid session bindings, want exactly one", validBindings)
	}
}

func TestConcurrentDistinctJoinsRespectPlayerLimit(t *testing.T) {
	registry := match.NewRegistry()
	room, err := registry.Create("host-session", "Host")
	if err != nil {
		t.Fatal(err)
	}

	const attempts = 24
	start := make(chan struct{})
	results := make(chan error, attempts)
	var workers sync.WaitGroup
	for i := 0; i < attempts; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			_, err := registry.Join(room.Code, fmt.Sprintf("session-%02d", i), fmt.Sprintf("Player %02d", i))
			results <- err
		}(i)
	}
	close(start)
	workers.Wait()
	close(results)

	joined := 0
	for err := range results {
		switch {
		case err == nil:
			joined++
		case errors.Is(err, match.ErrRoomFull):
		default:
			t.Fatalf("concurrent distinct join error = %v, want success or ErrRoomFull", err)
		}
	}
	if joined != match.MaxPlayers-1 {
		t.Fatalf("successful joins = %d, want %d", joined, match.MaxPlayers-1)
	}

	final, err := registry.Snapshot(room.Code, "host-session")
	if err != nil {
		t.Fatal(err)
	}
	if len(final.Players) != match.MaxPlayers {
		t.Fatalf("room contains %d players, want MaxPlayers (%d)", len(final.Players), match.MaxPlayers)
	}

	fullPlayer := final.Players[1]
	reconnected, err := registry.Join(room.Code, "full-room-reconnect", fullPlayer.Username)
	if err != nil {
		t.Fatalf("existing player could not reconnect to full room: %v", err)
	}
	if reconnected.Viewer.ID != fullPlayer.ID || len(reconnected.Players) != match.MaxPlayers {
		t.Fatalf("full-room reconnect changed identity or player count: viewer=%q players=%d", reconnected.Viewer.ID, len(reconnected.Players))
	}
}

func TestSessionRebindingRetainsPreviousPlayer(t *testing.T) {
	registry := match.NewRegistry()
	originalSession := "shared-session"
	original, err := registry.Create(originalSession, "First Name")
	if err != nil {
		t.Fatal(err)
	}

	current, err := registry.Join(original.Code, originalSession, "Different Name")
	if err != nil {
		t.Fatalf("same session could not join under a different username: %v", err)
	}
	if len(current.Players) != 2 {
		t.Fatalf("participants after rebind = %d, want original and new participant", len(current.Players))
	}
	if current.Viewer.Username != "Different Name" || current.Viewer.ID == original.Viewer.ID {
		t.Fatalf("current viewer = (%q, %q), want new participant distinct from original (%q)", current.Viewer.Username, current.Viewer.ID, original.Viewer.ID)
	}
	if current.HostPlayerID != original.Viewer.ID {
		t.Fatalf("host changed on rebind: got %q, want original player %q", current.HostPlayerID, original.Viewer.ID)
	}

	if err := registry.Leave(original.Code, originalSession); err != nil {
		t.Fatalf("leaving current participant failed: %v", err)
	}
	if _, err := registry.Snapshot(original.Code, originalSession); !errors.Is(err, match.ErrNotMember) {
		t.Fatalf("original session remained authorized after leaving current participant: %v", err)
	}

	reconnected, err := registry.Join(original.Code, "fresh-session", "first name")
	if err != nil {
		t.Fatalf("original username could not reconnect: %v", err)
	}
	if len(reconnected.Players) != 1 {
		t.Fatalf("participants after original reconnect = %d, want 1", len(reconnected.Players))
	}
	if reconnected.Viewer.ID != original.Viewer.ID {
		t.Fatalf("reconnected player ID = %q, want original %q", reconnected.Viewer.ID, original.Viewer.ID)
	}
	if reconnected.Viewer.Player != original.Viewer.Player {
		t.Fatalf("reconnected player state = %+v, want original state %+v", reconnected.Viewer.Player, original.Viewer.Player)
	}
	if reconnected.HostPlayerID != original.Viewer.ID {
		t.Fatalf("host after original reconnect = %q, want original player %q", reconnected.HostPlayerID, original.Viewer.ID)
	}
}

func TestMembershipHostAndMatchStatusAuthorization(t *testing.T) {
	registry := match.NewRegistry()
	host, err := registry.Create("host-session", "Host")
	if err != nil {
		t.Fatal(err)
	}
	member, err := registry.Join(host.Code, "member-session", "Member")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := registry.Snapshot(host.Code, "stranger"); !errors.Is(err, match.ErrNotMember) {
		t.Fatalf("stranger Snapshot error = %v, want ErrNotMember", err)
	}
	if _, err := registry.Start(host.Code, "member-session"); !errors.Is(err, match.ErrNotHost) {
		t.Fatalf("member Start error = %v, want ErrNotHost", err)
	}
	if _, err := registry.Finish(host.Code, "member-session"); !errors.Is(err, match.ErrNotHost) {
		t.Fatalf("member Finish error = %v, want ErrNotHost", err)
	}
	if _, err := registry.Move(host.Code, "stranger", "north"); !errors.Is(err, match.ErrNotMember) {
		t.Fatalf("stranger Move error = %v, want ErrNotMember", err)
	}
	if _, err := registry.Move(host.Code, "host-session", "not-a-direction"); !errors.Is(err, match.ErrNotActive) {
		t.Fatalf("lobby Move error = %v, want ErrNotActive", err)
	}
	if _, err := registry.Finish(host.Code, "stranger"); !errors.Is(err, match.ErrNotMember) {
		t.Fatalf("stranger Finish error = %v, want ErrNotMember", err)
	}

	active, err := registry.Start(host.Code, "host-session")
	if err != nil {
		t.Fatal(err)
	}
	if active.Status != match.Active {
		t.Fatalf("Start status = %v, want Active", active.Status)
	}
	if _, err := registry.Move(host.Code, "host-session", "not-a-direction"); !errors.Is(err, match.ErrInvalidMove) {
		t.Fatalf("invalid active move error = %v, want ErrInvalidMove", err)
	}
	if _, err := registry.Join(host.Code, "late-session", "Late"); !errors.Is(err, match.ErrGameStarted) {
		t.Fatalf("new join after Start error = %v, want ErrGameStarted", err)
	}
	if _, err := registry.Join(host.Code, "member-reconnect", "MEMBER"); err != nil {
		t.Fatalf("existing member could not reconnect to active match: %v", err)
	}

	finished, err := registry.Finish(host.Code, "host-session")
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != match.Finished {
		t.Fatalf("Finish status = %v, want Finished", finished.Status)
	}
	if _, err := registry.Join(host.Code, "another-late-session", "Another"); !errors.Is(err, match.ErrGameStarted) {
		t.Fatalf("new join after Finish error = %v, want ErrGameStarted", err)
	}
	if _, err := registry.Join(host.Code, "member-reconnect-after-finish", "member"); err != nil {
		t.Fatalf("existing member could not reconnect to finished match: %v", err)
	}
	if _, err := registry.Snapshot(host.Code, "stranger"); !errors.Is(err, match.ErrNotMember) {
		t.Fatalf("stranger Snapshot after Finish error = %v, want ErrNotMember", err)
	}
	if member.Viewer.ID == "" {
		t.Fatal("joined member has empty player ID")
	}
}

func TestSnapshotsDoNotExposeMutableMatchState(t *testing.T) {
	registry := match.NewRegistry()
	created, err := registry.Create("snapshot-session", "Snapshotter")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := registry.Snapshot(created.Code, "snapshot-session")
	if err != nil {
		t.Fatal(err)
	}
	returned, err := registry.Snapshot(created.Code, "snapshot-session")
	if err != nil {
		t.Fatal(err)
	}

	roomPos := returned.Viewer.Player.Position
	room := returned.World.Rooms[roomPos]
	if room == nil {
		t.Fatalf("player room %v missing from snapshot", roomPos)
	}
	room.NWall.Type = "snapshot-corruption"
	if len(room.Items) > 0 {
		room.Items[0].Type = "snapshot-corruption"
	} else {
		room.Items = append(room.Items, game.Item{Type: "snapshot-corruption"})
	}
	delete(returned.World.Explored, roomPos)
	returned.World.Explored[game.Pos{X: -1, Y: -1}] = &game.Room{EWall: game.Wall{Type: "snapshot-corruption"}}
	returned.Players[0].Username = "snapshot-corruption"
	returned.Players[0].Player.Health = -1
	returned.Viewer.ID = "snapshot-corruption"
	returned.Viewer.Player.Position = game.Pos{X: -1, Y: -1}

	actual, err := registry.Snapshot(created.Code, "snapshot-session")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("mutating returned snapshot changed later match state")
	}
}

func TestLeaveTransfersHostAndFinalLeaveRemovesRoom(t *testing.T) {
	registry := match.NewRegistry()
	host, err := registry.Create("host-session", "Host")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Join(host.Code, "member-a", "A"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Join(host.Code, "member-b", "B"); err != nil {
		t.Fatal(err)
	}

	if err := registry.Leave(host.Code, "host-session"); err != nil {
		t.Fatal(err)
	}
	remaining, err := registry.Snapshot(host.Code, "member-a")
	if err != nil {
		t.Fatal(err)
	}
	players := append([]match.ParticipantView(nil), remaining.Players...)
	sort.Slice(players, func(i, j int) bool {
		return strings.ToLower(strings.TrimSpace(players[i].Username)) < strings.ToLower(strings.TrimSpace(players[j].Username))
	})
	if remaining.HostPlayerID != players[0].ID {
		t.Fatalf("host transferred to %q, want first sorted remaining player %q", remaining.HostPlayerID, players[0].ID)
	}
	for _, player := range remaining.Players {
		if player.ID == host.Viewer.ID {
			t.Fatal("departed host remains in participant list")
		}
	}

	if err := registry.Leave(host.Code, "member-a"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Leave(host.Code, "member-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Snapshot(host.Code, "member-b"); !errors.Is(err, match.ErrRoomNotFound) {
		t.Fatalf("Snapshot after final leave error = %v, want ErrRoomNotFound", err)
	}
}

func TestCleanupExpiresIdleMatch(t *testing.T) {
	registry := match.NewRegistry()
	room, err := registry.Create("idle-session", "Idle")
	if err != nil {
		t.Fatal(err)
	}
	registry.Cleanup(time.Now().Add(48*time.Hour), time.Hour)
	if _, err := registry.Snapshot(room.Code, "idle-session"); !errors.Is(err, match.ErrRoomNotFound) && !errors.Is(err, match.ErrClosed) {
		t.Fatalf("Snapshot after cleanup error = %v, want ErrRoomNotFound or ErrClosed", err)
	}
	if err := registry.Leave(room.Code, "idle-session"); !errors.Is(err, match.ErrRoomNotFound) && !errors.Is(err, match.ErrClosed) {
		t.Fatalf("Leave stale match error = %v, want ErrRoomNotFound or ErrClosed", err)
	}
}

func TestOnlyAuthorizedSnapshotsRefreshMatchActivity(t *testing.T) {
	authorizedRegistry := match.NewRegistry()
	authorizedRoom, err := authorizedRegistry.Create("authorized-session", "Authorized")
	if err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now()
	if _, err := authorizedRegistry.Snapshot(authorizedRoom.Code, "authorized-session"); err != nil {
		t.Fatal(err)
	}
	authorizedRegistry.Cleanup(cutoff, 0)
	if _, err := authorizedRegistry.Snapshot(authorizedRoom.Code, "authorized-session"); err != nil {
		t.Fatalf("authorized poll did not keep match active: %v", err)
	}

	unauthorizedRegistry := match.NewRegistry()
	unauthorizedRoom, err := unauthorizedRegistry.Create("member-session", "Member")
	if err != nil {
		t.Fatal(err)
	}
	cutoff = time.Now()
	if _, err := unauthorizedRegistry.Snapshot(unauthorizedRoom.Code, "stranger"); !errors.Is(err, match.ErrNotMember) {
		t.Fatalf("stranger Snapshot error = %v, want ErrNotMember", err)
	}
	unauthorizedRegistry.Cleanup(cutoff, 0)
	if _, err := unauthorizedRegistry.Snapshot(unauthorizedRoom.Code, "member-session"); !errors.Is(err, match.ErrRoomNotFound) {
		t.Fatalf("unauthorized poll refreshed match activity; Snapshot after cleanup error = %v", err)
	}
}

func TestInvalidUsernamesAreRejected(t *testing.T) {
	registry := match.NewRegistry()
	for _, username := range []string{"", " \t ", "name\ncontrol", "nul\x00control", strings.Repeat("界", 33)} {
		t.Run(fmt.Sprintf("create_%d_%q", len([]rune(username)), username), func(t *testing.T) {
			if _, err := registry.Create("session-"+fmt.Sprint(len(username)), username); !errors.Is(err, match.ErrInvalidUsername) {
				t.Fatalf("Create(%q) error = %v, want ErrInvalidUsername", username, err)
			}
		})
	}
	room, err := registry.Create("valid-session", "Valid")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Join(room.Code, "invalid-join", "\t"); !errors.Is(err, match.ErrInvalidUsername) {
		t.Fatalf("Join with blank username error = %v, want ErrInvalidUsername", err)
	}
}

func createStartedMatchWithMove(t *testing.T, registry *match.Registry, sessionPrefix string) (match.Snapshot, string) {
	t.Helper()
	for attempt := 0; attempt < 64; attempt++ {
		sessionID := fmt.Sprintf("%s-%02d", sessionPrefix, attempt)
		room, err := registry.Create(sessionID, "  Walker  ")
		if err != nil {
			t.Fatal(err)
		}
		started, err := registry.Start(room.Code, sessionID)
		if err != nil {
			t.Fatal(err)
		}
		for _, direction := range possibleDirections(started.Moves) {
			moved, err := registry.Move(room.Code, sessionID, direction)
			if err == nil {
				if moved.Viewer.Player.Position == started.Viewer.Player.Position {
					t.Fatalf("Move(%q) succeeded without changing position", direction)
				}
				if len(moved.World.Explored) <= len(started.World.Explored) {
					t.Fatalf("moving to adjacent room did not expand exploration: before=%d after=%d", len(started.World.Explored), len(moved.World.Explored))
				}
				return moved, sessionID
			}
			if !errors.Is(err, match.ErrInvalidMove) {
				t.Fatalf("Move(%q) error = %v, want success or ErrInvalidMove", direction, err)
			}
		}
		if _, err := registry.Move(room.Code, sessionID, "not-a-direction"); !errors.Is(err, match.ErrInvalidMove) {
			t.Fatalf("invalid direction error = %v, want ErrInvalidMove", err)
		}
		if err := registry.Leave(room.Code, sessionID); err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("could not generate a start room with a valid move after 64 bounded attempts")
	return match.Snapshot{}, ""
}

func possibleDirections(moves game.PossibleMoves) []string {
	directions := make([]string, 0, 4)
	if moves.North.Possible {
		directions = append(directions, "north")
	}
	if moves.South.Possible {
		directions = append(directions, "south")
	}
	if moves.East.Possible {
		directions = append(directions, "east")
	}
	if moves.West.Possible {
		directions = append(directions, "west")
	}
	return directions
}
