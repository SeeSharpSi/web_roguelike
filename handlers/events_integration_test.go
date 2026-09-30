package handlers_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

type serverEvent struct {
	event string
	id    string
	data  string
}

func startEventServer(t *testing.T, app *roomTestApp) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(app.router)
	t.Cleanup(server.Close)
	return server
}

func openEventStream(t *testing.T, server *httptest.Server, client *roomTestClient, code, lastEventID string) (*http.Response, *bufio.Reader) {
	t.Helper()
	return openEventStreamContext(t, context.Background(), server, client, code, lastEventID)
}

func openEventStreamContext(t *testing.T, ctx context.Context, server *httptest.Server, client *roomTestClient, code, lastEventID string) (*http.Response, *bufio.Reader) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/rooms/"+code+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	if client.cookie != nil {
		request.AddCookie(client.cookie)
	}
	if lastEventID != "" {
		request.Header.Set("Last-Event-ID", lastEventID)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open room event stream: %v", err)
	}
	t.Cleanup(func() { response.Body.Close() })
	if response.StatusCode != http.StatusOK {
		t.Fatalf("event stream status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Header.Get("Content-Type"); got != "text/event-stream;charset=utf-8" {
		t.Errorf("event stream Content-Type = %q", got)
	}
	if got := response.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("event stream Cache-Control = %q, want no-store", got)
	}
	if got := response.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("event stream X-Accel-Buffering = %q, want no", got)
	}
	return response, bufio.NewReader(response.Body)
}

func readServerEvent(t *testing.T, reader *bufio.Reader) serverEvent {
	t.Helper()
	var result serverEvent
	var data []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE frame: %v", err)
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			if len(data) == 0 && result.event == "" && result.id == "" {
				continue
			}
			result.data = strings.Join(data, "\n")
			return result
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			value = ""
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			result.event = value
		case "id":
			result.id = value
		case "data":
			data = append(data, value)
		}
	}
}

func TestRoomEventsInitialSnapshotLatestRevisionAndPeerUpdates(t *testing.T) {
	app := newRoomTestApp()
	host := app.client()
	code, _ := createMovableRoom(t, app, host, "Host")
	guest := app.client()
	guest.get("/")
	if response := guest.post("/rooms/"+code+"/join", mapValues("username", "Guest")); response.Code != http.StatusSeeOther {
		t.Fatalf("guest join status = %d; body=%s", response.Code, response.Body.String())
	}

	latest := mustSnapshot(t, app, code, host.sessionID(t))
	server := startEventServer(t, app)
	_, hostReader := openEventStream(t, server, host, code, "1")
	initial := readServerEvent(t, hostReader)
	if initial.event != "" {
		t.Fatalf("initial event name = %q, want unnamed event", initial.event)
	}
	if initial.id != strconv.FormatUint(latest.Revision, 10) {
		t.Fatalf("initial event ID = %q, want latest revision %d", initial.id, latest.Revision)
	}
	if !strings.Contains(initial.data, `id="room-state"`) || !strings.Contains(initial.data, `data-status="lobby"`) {
		t.Fatalf("initial SSE data missing room state markup: %s", initial.data)
	}
	hostPlayer := participantByUsername(t, latest, "Host")
	guestPlayer := participantByUsername(t, latest, "Guest")
	if !strings.Contains(initial.data, fmt.Sprintf(`data-player-id="%s"`, hostPlayer.ID)) {
		t.Fatalf("host-specific initial roster missing host player %s", hostPlayer.ID)
	}
	if !strings.Contains(initial.data, fmt.Sprintf(`data-player-id="%s"`, guestPlayer.ID)) {
		t.Fatalf("initial roster missing guest player %s", guestPlayer.ID)
	}
	if !playerRowContains(initial.data, hostPlayer.ID, `<span class="you-badge">You</span>`) {
		t.Fatal("host initial event did not mark host row as viewer")
	}
	_, guestReader := openEventStream(t, server, guest, code, "1")
	guestInitial := readServerEvent(t, guestReader)
	guestSnapshot := mustSnapshot(t, app, code, guest.sessionID(t))
	if guestInitial.id != strconv.FormatUint(guestSnapshot.Revision, 10) {
		t.Fatalf("guest initial event ID = %q, want latest revision %d despite Last-Event-ID", guestInitial.id, guestSnapshot.Revision)
	}
	if !playerRowContains(guestInitial.data, guestPlayer.ID, `<span class="you-badge">You</span>`) {
		t.Fatal("guest initial event did not mark guest row as viewer")
	}

	start := host.post("/rooms/"+code+"/start", mapValues())
	if start.Code != http.StatusSeeOther {
		t.Fatalf("start status = %d; body=%s", start.Code, start.Body.String())
	}
	startedEvent := readServerEvent(t, hostReader)
	if startedEvent.event != "" || !strings.Contains(startedEvent.data, `data-status="active"`) {
		t.Fatalf("start update = event %q data %s", startedEvent.event, startedEvent.data)
	}
	if startedEvent.id == initial.id {
		t.Fatalf("start event revision %q did not advance from %q", startedEvent.id, initial.id)
	}

	beforeMove := mustSnapshot(t, app, code, guest.sessionID(t))
	direction := possibleDirection(beforeMove)
	if direction == "" {
		t.Fatal("guest has no possible move in active room")
	}
	move := guest.post("/rooms/"+code+"/actions", mapValues("direction", direction))
	if move.Code != http.StatusSeeOther {
		t.Fatalf("guest move status = %d; body=%s", move.Code, move.Body.String())
	}
	afterMove := mustSnapshot(t, app, code, host.sessionID(t))
	movedPlayer := participantByUsername(t, afterMove, "Guest")
	if movedPlayer.Player.Position == guestPlayer.Player.Position {
		t.Fatalf("guest position did not change after %s", direction)
	}
	if afterMove.World.Explored[movedPlayer.Player.Position] == nil || len(afterMove.World.Explored) <= len(latest.World.Explored) {
		t.Fatal("guest movement did not reveal a new room in the shared map")
	}
	moveEvent := readServerEvent(t, hostReader)
	if moveEvent.event != "" || moveEvent.id != strconv.FormatUint(afterMove.Revision, 10) {
		t.Fatalf("movement event = name %q revision %q, want revision %d", moveEvent.event, moveEvent.id, afterMove.Revision)
	}
	for _, attribute := range []string{
		fmt.Sprintf(`data-player-id="%s"`, movedPlayer.ID),
		fmt.Sprintf(`data-x="%d"`, movedPlayer.Player.Position.X),
		fmt.Sprintf(`data-y="%d"`, movedPlayer.Player.Position.Y),
		`class="room-cell charted-room`,
	} {
		if !strings.Contains(moveEvent.data, attribute) {
			t.Errorf("movement update missing %q", attribute)
		}
	}
}

func TestRoomEventsRecheckAuthorizationAndRedirectOnTakeover(t *testing.T) {
	app := newRoomTestApp()
	host := app.client()
	code, _ := createRoom(t, host, "Host")
	oldGuest := app.client()
	oldGuest.get("/")
	if response := oldGuest.post("/rooms/"+code+"/join", mapValues("username", "Guest")); response.Code != http.StatusSeeOther {
		t.Fatalf("guest join status = %d", response.Code)
	}

	server := startEventServer(t, app)
	_, reader := openEventStream(t, server, oldGuest, code, "")
	readServerEvent(t, reader)

	replacement := app.client()
	replacement.get("/")
	if response := replacement.post("/rooms/"+code+"/join", mapValues("username", "Guest")); response.Code != http.StatusSeeOther {
		t.Fatalf("guest reconnect status = %d; body=%s", response.Code, response.Body.String())
	}
	redirect := readServerEvent(t, reader)
	if redirect.event != "room-redirect" || redirect.data != "/rooms/"+code {
		t.Fatalf("takeover event = %+v, want redirect to /rooms/%s", redirect, code)
	}
	if _, err := reader.ReadString('\n'); err != io.EOF {
		t.Fatalf("stream read after takeover redirect error = %v, want EOF", err)
	}
}

func TestRoomEventsRedirectWhenRoomCleanupClosesMatch(t *testing.T) {
	app := newRoomTestApp()
	host := app.client()
	code, _ := createRoom(t, host, "Host")
	server := startEventServer(t, app)
	_, reader := openEventStream(t, server, host, code, "")
	readServerEvent(t, reader)

	app.handler.Matches.Cleanup(time.Now().Add(3*time.Hour), 2*time.Hour)
	redirect := readServerEvent(t, reader)
	if redirect.event != "room-redirect" || redirect.data != "/rooms/"+code {
		t.Fatalf("cleanup event = %+v, want redirect to /rooms/%s", redirect, code)
	}
	if _, err := reader.ReadString('\n'); err != io.EOF {
		t.Fatalf("stream read after cleanup redirect error = %v, want EOF", err)
	}
}

func TestRoomEventsRejectStrangersWithoutCreatingSessionsOrLeakingState(t *testing.T) {
	app := newRoomTestApp()
	host := app.client()
	code, _ := createRoom(t, host, "Host")
	server := startEventServer(t, app)

	response, err := http.Get(server.URL + "/rooms/" + code + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("stranger status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
	if response.Header.Get("Set-Cookie") != "" {
		t.Fatalf("stranger event request created session cookie %q", response.Header.Get("Set-Cookie"))
	}
	for _, fragment := range []string{`id="room-state"`, `class="map-grid"`, `data-player-id=`} {
		if strings.Contains(string(body), fragment) {
			t.Errorf("unauthorized event response leaked %q", fragment)
		}
	}
}

func TestRoomEventsReturnNotFoundForMissingRoom(t *testing.T) {
	app := newRoomTestApp()
	host := app.client()
	host.get("/")
	server := startEventServer(t, app)
	request, err := http.NewRequest(http.MethodGet, server.URL+"/rooms/NOTREAL/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(host.cookie)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing-room event status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `id="room-state"`) {
		t.Fatalf("missing-room response exposed game state: %s", body)
	}
}

func TestRoomEventsRequireFlushSupportBeforeStreaming(t *testing.T) {
	app := newRoomTestApp()
	host := app.client()
	code, _ := createRoom(t, host, "Host")
	request := httptest.NewRequest(http.MethodGet, "/rooms/"+code+"/events", nil)
	request.AddCookie(host.cookie)
	response := &nonFlushingResponseWriter{header: make(http.Header)}
	app.router.ServeHTTP(response, request)
	if response.status != http.StatusInternalServerError {
		t.Fatalf("non-flushing writer status = %d, want %d", response.status, http.StatusInternalServerError)
	}
	if bytes.Contains(response.body.Bytes(), []byte(`id="room-state"`)) {
		t.Fatal("non-flushing writer received room data")
	}
}

type nonFlushingResponseWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *nonFlushingResponseWriter) Header() http.Header {
	return w.header
}

func (w *nonFlushingResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *nonFlushingResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(body)
}

func TestRoomEventsCancellationReturnsHandler(t *testing.T) {
	app := newRoomTestApp()
	host := app.client()
	code, _ := createRoom(t, host, "Host")
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		app.router.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	response, reader := openEventStreamContext(t, ctx, server, host, code, "")
	readServerEvent(t, reader)
	cancel()
	response.Body.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("event handler did not return after client cancellation")
	}
}

func mapValues(values ...string) url.Values {
	form := make(url.Values, len(values)/2)
	for index := 0; index+1 < len(values); index += 2 {
		form[values[index]] = []string{values[index+1]}
	}
	return form
}

func playerRowContains(markup, playerID, fragment string) bool {
	attribute := fmt.Sprintf(`data-player-id="%s"`, playerID)
	searchFrom := 0
	for {
		position := strings.Index(markup[searchFrom:], attribute)
		if position < 0 {
			return false
		}
		position += searchFrom
		searchFrom = position + len(attribute)
		rowStart := strings.LastIndex(markup[:position], "<li ")
		rowEnd := strings.Index(markup[position:], "</li>")
		if rowStart >= 0 && rowEnd >= 0 && strings.Contains(markup[rowStart:position+rowEnd], fragment) {
			return true
		}
	}
}
