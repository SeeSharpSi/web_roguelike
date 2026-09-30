package match

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"seesharpsi/web_roguelike/game"
)

func TestWatchBroadcastsEachMatchRevision(t *testing.T) {
	registry, code := notificationFixture()
	host, change := watchSet(t, registry, code, "host-session", "member-session")

	assertChange(t, registry, code, host, change, "host-session", func() error {
		_, err := registry.Join(code, "third-session", "Third")
		return err
	})
	host, change = watchSet(t, registry, code, "host-session", "member-session", "third-session")
	started, nextChange := assertChange(t, registry, code, host, change, "host-session", func() error {
		_, err := registry.Start(code, "host-session")
		return err
	})
	change = nextChange
	if started.Status != Active {
		t.Fatalf("Start status = %q, want %q", started.Status, Active)
	}

	host, change = watchSet(t, registry, code, "host-session", "member-session", "third-session")
	_, change = assertChange(t, registry, code, host, change, "host-session", func() error {
		_, err := registry.Move(code, "host-session", "east")
		return err
	})

	host, change = watchSet(t, registry, code, "host-session", "member-session", "third-session")
	finished, change := assertChange(t, registry, code, host, change, "host-session", func() error {
		_, err := registry.Finish(code, "host-session")
		return err
	})
	if finished.Status != Finished {
		t.Fatalf("Finish status = %q, want %q", finished.Status, Finished)
	}

	host, change = watchSet(t, registry, code, "host-session", "member-session", "third-session")
	transferred, change := assertChange(t, registry, code, host, change, "member-session", func() error {
		return registry.Leave(code, "host-session")
	})
	if transferred.HostPlayerID != transferred.Viewer.ID {
		t.Fatalf("host transfer selected %q, want remaining member %q", transferred.HostPlayerID, transferred.Viewer.ID)
	}

	member, change := watchSet(t, registry, code, "member-session", "third-session")
	_, change = assertChange(t, registry, code, member, change, "member-session", func() error {
		return registry.Leave(code, "third-session")
	})

	remaining, change := watchSet(t, registry, code, "member-session")
	if err := registry.Leave(code, "member-session"); err != nil {
		t.Fatal(err)
	}
	assertClosed(t, change)
	if remaining.Revision != 7 {
		t.Fatalf("revision before final Leave = %d, want 7", remaining.Revision)
	}
	if _, _, err := registry.Watch(code, "member-session"); !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("Watch after final Leave error = %v, want ErrRoomNotFound", err)
	}
}

func TestWatchDoesNotNotifyRejectedOrIdempotentOperations(t *testing.T) {
	registry, code := notificationFixture()
	snapshot, change := watchSet(t, registry, code, "host-session", "member-session")

	if _, err := registry.Join(code, "member-session", "MEMBER"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Start(code, "member-session"); !errors.Is(err, ErrNotHost) {
		t.Fatalf("non-host Start error = %v, want ErrNotHost", err)
	}
	if _, err := registry.Move(code, "host-session", "east"); !errors.Is(err, ErrNotActive) {
		t.Fatalf("lobby Move error = %v, want ErrNotActive", err)
	}
	if err := registry.Leave(code, "stranger"); !errors.Is(err, ErrNotMember) {
		t.Fatalf("stranger Leave error = %v, want ErrNotMember", err)
	}
	if _, _, err := registry.Watch(code, "stranger"); !errors.Is(err, ErrNotMember) {
		t.Fatalf("stranger Watch error = %v, want ErrNotMember", err)
	}
	assertStillWatching(t, registry, code, snapshot, change)

	started, change := assertChange(t, registry, code, snapshot, change, "host-session", func() error {
		_, err := registry.Start(code, "host-session")
		return err
	})
	if _, err := registry.Start(code, "host-session"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Move(code, "host-session", "not-a-direction"); !errors.Is(err, ErrInvalidMove) {
		t.Fatalf("invalid Move error = %v, want ErrInvalidMove", err)
	}
	if _, err := registry.Finish(code, "member-session"); !errors.Is(err, ErrNotHost) {
		t.Fatalf("non-host Finish error = %v, want ErrNotHost", err)
	}
	if _, err := registry.Join(code, "late-session", "Late"); !errors.Is(err, ErrGameStarted) {
		t.Fatalf("late Join error = %v, want ErrGameStarted", err)
	}
	assertStillWatching(t, registry, code, started, change)
}

func TestWatchReconnectNotifiesAndRevokesPreviousSession(t *testing.T) {
	registry, code := notificationFixture()
	previous, change := watchSet(t, registry, code, "host-session", "member-session")
	_, nextChange := assertChange(t, registry, code, previous, change, "host-session", func() error {
		_, err := registry.Join(code, "replacement-session", " member ")
		return err
	})
	reconnected, _, err := registry.Watch(code, "replacement-session")
	if err != nil {
		t.Fatal(err)
	}
	if reconnected.Viewer.Username != "Member" {
		t.Fatalf("reconnected username = %q, want Member", reconnected.Viewer.Username)
	}
	if _, _, err := registry.Watch(code, "member-session"); !errors.Is(err, ErrNotMember) {
		t.Fatalf("revoked Watch error = %v, want ErrNotMember", err)
	}
	if _, err := registry.Join(code, "replacement-session", "MEMBER"); err != nil {
		t.Fatal(err)
	}
	assertStillWatching(t, registry, code, reconnected, nextChange)
}

func TestWatchSnapshotRemainsIndependent(t *testing.T) {
	registry, code := notificationFixture()
	snapshot, _, err := registry.Watch(code, "host-session")
	if err != nil {
		t.Fatal(err)
	}
	snapshot.World.Rooms[game.Pos{X: 0, Y: 0}].EWall.Type = "mutated"
	snapshot.World.Rooms[game.Pos{X: 0, Y: 0}].Items = append(snapshot.World.Rooms[game.Pos{X: 0, Y: 0}].Items, game.Item{Type: "mutated"})
	delete(snapshot.World.Rooms, game.Pos{X: 1, Y: 0})
	snapshot.Players[0].Username = "mutated"
	snapshot.Viewer.ID = "mutated"

	actual, _, err := registry.Watch(code, "host-session")
	if err != nil {
		t.Fatal(err)
	}
	if actual.Viewer.ID != "host" || actual.World.Rooms[game.Pos{X: 0, Y: 0}].EWall.Type != game.WallEmpty {
		t.Fatal("mutating a Watch snapshot changed live match state")
	}
	if actual.World.Rooms[game.Pos{X: 1, Y: 0}] == nil {
		t.Fatal("mutating a Watch snapshot removed a live map room")
	}
}

func TestWatchWakesOnCleanupAndClosedRoomCannotBeWatched(t *testing.T) {
	registry, code := notificationFixture()
	_, change := watchSet(t, registry, code, "host-session", "member-session")
	registry.Cleanup(time.Now().Add(48*time.Hour), time.Hour)
	assertClosed(t, change)
	if _, _, err := registry.Watch(code, "host-session"); !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("Watch after Cleanup error = %v, want ErrRoomNotFound", err)
	}
}

func TestConcurrentWatchAndJoinCapturesConsistentRevisions(t *testing.T) {
	registry, code := notificationFixture()
	type observation struct {
		snapshot Snapshot
		changed  <-chan struct{}
		err      error
	}
	const watcherCount = 4
	const watchesPerWorker = 100
	observations := make(chan observation, watcherCount*watchesPerWorker)
	mutationErrors := make(chan error, 5)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < watcherCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for range watchesPerWorker {
				snapshot, changed, err := registry.Watch(code, "host-session")
				observations <- observation{snapshot: snapshot, changed: changed, err: err}
			}
		}()
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 5; i++ {
			_, err := registry.Join(code, fmt.Sprintf("joined-%d", i), fmt.Sprintf("Joined %d", i))
			mutationErrors <- err
		}
	}()
	close(start)
	workers.Wait()
	close(observations)
	close(mutationErrors)
	for err := range mutationErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	final, finalChange, err := registry.Watch(code, "host-session")
	if err != nil {
		t.Fatal(err)
	}
	if final.Revision != 6 {
		t.Fatalf("final revision = %d, want 6", final.Revision)
	}
	for observed := range observations {
		if observed.err != nil {
			t.Fatal(observed.err)
		}
		switch {
		case observed.snapshot.Revision < final.Revision:
			assertClosed(t, observed.changed)
		case observed.snapshot.Revision == final.Revision:
			if observed.changed != finalChange {
				t.Fatal("Watch returned current revision with stale broadcast channel")
			}
			assertOpen(t, observed.changed)
		default:
			t.Fatalf("observed future revision %d after final revision %d", observed.snapshot.Revision, final.Revision)
		}
	}
}

func notificationFixture() (*Registry, string) {
	code := "ABC123"
	host := &participant{
		id:              "host",
		username:        "Host",
		usernameKey:     "host",
		player:          game.Player{Alive: true, Position: game.Pos{X: 0, Y: 0}},
		activeSessionID: "host-session",
	}
	member := &participant{
		id:              "member",
		username:        "Member",
		usernameKey:     "member",
		player:          game.Player{Alive: true, Position: game.Pos{X: 0, Y: 0}},
		activeSessionID: "member-session",
	}
	start := &game.Room{EWall: game.Wall{Type: game.WallEmpty}, Items: []game.Item{{Type: "start"}}}
	next := &game.Room{WWall: game.Wall{Type: game.WallEmpty}}
	world := game.Map{
		Width:    2,
		Length:   1,
		StartPos: game.Pos{X: 0, Y: 0},
		Rooms: map[game.Pos]*game.Room{
			{X: 0, Y: 0}: start,
			{X: 1, Y: 0}: next,
		},
		Explored: map[game.Pos]*game.Room{{X: 0, Y: 0}: start},
	}
	room := &match{
		changed:       make(chan struct{}),
		code:          code,
		status:        Lobby,
		hostPlayerID:  host.id,
		players:       map[string]*participant{host.id: host, member.id: member},
		usernameIndex: map[string]string{host.usernameKey: host.id, member.usernameKey: member.id},
		world:         world,
		revision:      1,
		lastActivity:  time.Now(),
	}
	return &Registry{rooms: map[string]*match{code: room}}, code
}

func watchSet(t *testing.T, registry *Registry, code string, sessions ...string) (Snapshot, <-chan struct{}) {
	t.Helper()
	var snapshot Snapshot
	var changed <-chan struct{}
	for i, sessionID := range sessions {
		current, currentChange, err := registry.Watch(code, sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			snapshot, changed = current, currentChange
			continue
		}
		if current.Revision != snapshot.Revision || currentChange != changed {
			t.Fatal("viewers did not receive same revision and broadcast channel")
		}
	}
	return snapshot, changed
}

func assertChange(t *testing.T, registry *Registry, code string, before Snapshot, changed <-chan struct{}, sessionID string, action func() error) (Snapshot, <-chan struct{}) {
	t.Helper()
	if err := action(); err != nil {
		t.Fatal(err)
	}
	assertClosed(t, changed)
	after, nextChange, err := registry.Watch(code, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision+1 {
		t.Fatalf("revision after change = %d, want %d", after.Revision, before.Revision+1)
	}
	if nextChange == changed {
		t.Fatal("Watch returned previous broadcast channel after revision change")
	}
	assertOpen(t, nextChange)
	return after, nextChange
}

func assertStillWatching(t *testing.T, registry *Registry, code string, before Snapshot, changed <-chan struct{}) {
	t.Helper()
	assertOpen(t, changed)
	after, currentChange, err := registry.Watch(code, "host-session")
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || currentChange != changed {
		t.Fatalf("unchanged operation altered revision/channel: got revision %d, want %d", after.Revision, before.Revision)
	}
}

func assertClosed(t *testing.T, changed <-chan struct{}) {
	t.Helper()
	select {
	case <-changed:
	default:
		t.Fatal("expected broadcast channel to be closed")
	}
}

func assertOpen(t *testing.T, changed <-chan struct{}) {
	t.Helper()
	select {
	case <-changed:
		t.Fatal("broadcast channel closed without a revision change")
	default:
	}
}
