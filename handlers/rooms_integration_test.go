package handlers_test

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"seesharpsi/web_roguelike/handlers"
	"seesharpsi/web_roguelike/match"
	"seesharpsi/web_roguelike/session"
)

type roomTestApp struct {
	handler *handlers.Handler
	router  http.Handler
}

type roomTestClient struct {
	app    *roomTestApp
	cookie *http.Cookie
}

func newRoomTestApp() *roomTestApp {
	h := &handlers.Handler{
		Manager: session.NewManager(),
		Matches: match.NewRegistry(),
	}
	return &roomTestApp{handler: h, router: h.Routes()}
}

func (app *roomTestApp) client() *roomTestClient {
	return &roomTestClient{app: app}
}

func (client *roomTestClient) request(method, path string, form url.Values, rawBody string, htmx bool) *httptest.ResponseRecorder {
	var body strings.Reader
	contentType := ""
	if form != nil {
		body = *strings.NewReader(form.Encode())
		contentType = "application/x-www-form-urlencoded"
	} else {
		body = *strings.NewReader(rawBody)
		if rawBody != "" {
			contentType = "application/x-www-form-urlencoded"
		}
	}
	request := httptest.NewRequest(method, path, &body)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if htmx {
		request.Header.Set("HX-Request", "true")
	}
	if client.cookie != nil {
		request.AddCookie(client.cookie)
	}
	recorder := httptest.NewRecorder()
	client.app.router.ServeHTTP(recorder, request)
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "session_id" {
			copy := *cookie
			client.cookie = &copy
			break
		}
	}
	return recorder
}

func (client *roomTestClient) get(path string) *httptest.ResponseRecorder {
	return client.request(http.MethodGet, path, nil, "", false)
}

func (client *roomTestClient) post(path string, form url.Values) *httptest.ResponseRecorder {
	return client.request(http.MethodPost, path, form, "", false)
}

func (client *roomTestClient) postHTMX(path string, form url.Values) *httptest.ResponseRecorder {
	return client.request(http.MethodPost, path, form, "", true)
}

func (client *roomTestClient) sessionID(t *testing.T) string {
	t.Helper()
	if client.cookie == nil || client.cookie.Value == "" {
		t.Fatal("request did not establish session_id cookie")
	}
	return client.cookie.Value
}

func createRoom(t *testing.T, client *roomTestClient, username string) (string, *httptest.ResponseRecorder) {
	t.Helper()
	client.get("/")
	response := client.post("/rooms", url.Values{"username": {username}})
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create room status = %d, want %d; body=%s", response.Code, http.StatusSeeOther, response.Body.String())
	}
	const prefix = "/rooms/"
	location := response.Header().Get("Location")
	if !strings.HasPrefix(location, prefix) {
		t.Fatalf("create room Location = %q, want canonical room path", location)
	}
	code := strings.TrimPrefix(location, prefix)
	if code == "" || code != match.NormalizeCode(code) {
		t.Fatalf("created room code %q is not canonical uppercase code", code)
	}
	return code, response
}

func createMovableRoom(t *testing.T, app *roomTestApp, client *roomTestClient, username string) (string, match.Snapshot) {
	t.Helper()
	const maxCandidates = 64
	for attempt := 1; attempt <= maxCandidates; attempt++ {
		code, _ := createRoom(t, client, username)
		snapshot := mustSnapshot(t, app, code, client.sessionID(t))
		if possibleDirection(snapshot) != "" {
			return code, snapshot
		}

		if err := app.handler.Matches.Leave(code, client.sessionID(t)); err != nil {
			t.Fatalf("remove unusable candidate room %s: %v", code, err)
		}
		if _, err := app.handler.Matches.Snapshot(code, client.sessionID(t)); !errors.Is(err, match.ErrRoomNotFound) {
			t.Fatalf("unusable candidate room %s remains after leave: snapshot error = %v, want ErrRoomNotFound", code, err)
		}
	}
	t.Fatalf("failed to create room with traversable start move after %d candidates", maxCandidates)
	return "", match.Snapshot{}
}

func mustSnapshot(t *testing.T, app *roomTestApp, code, sessionID string) match.Snapshot {
	t.Helper()
	snapshot, err := app.handler.Matches.Snapshot(code, sessionID)
	if err != nil {
		t.Fatalf("Snapshot(%q, session) error = %v", code, err)
	}
	return snapshot
}

func participantByUsername(t *testing.T, snapshot match.Snapshot, username string) match.ParticipantView {
	t.Helper()
	for _, participant := range snapshot.Players {
		if strings.EqualFold(participant.Username, username) {
			return participant
		}
	}
	t.Fatalf("participant %q not found in snapshot players %+v", username, snapshot.Players)
	return match.ParticipantView{}
}

func assertNoStore(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func assertStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, want, response.Body.String())
	}
}

func TestCreateRoomAndRenderAuthorizedRoom(t *testing.T) {
	app := newRoomTestApp()
	client := app.client()
	code, createResponse := createRoom(t, client, "Host")
	if got := createResponse.Header().Get("Location"); got != "/rooms/"+code {
		t.Fatalf("create Location = %q, want /rooms/%s", got, code)
	}
	if got := createResponse.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("create Cache-Control = %q, want no-store", got)
	}

	snapshot := mustSnapshot(t, app, code, client.sessionID(t))
	if snapshot.Status != match.Lobby {
		t.Fatalf("created room status = %q, want lobby", snapshot.Status)
	}
	if snapshot.Viewer.ID == "" || snapshot.Viewer.Username != "Host" {
		t.Fatalf("created viewer = %+v, want original host identity", snapshot.Viewer)
	}
	player := snapshot.Viewer.Player
	if player.Name == "" || player.Name == snapshot.Viewer.Username || player.Role == "" {
		t.Fatalf("created host character = %+v, want generated name separate from username and directive", player)
	}
	if player.Health < 83 || player.Health > 100 || player.Stamina < 50 || player.Stamina > 100 || player.Strength < 0.80 || player.Strength > 1.00 || !player.Alive {
		t.Fatalf("created host stats are outside generated ranges: %+v", player)
	}
	if len(snapshot.Players) != 1 || snapshot.Players[0].ID != snapshot.Viewer.ID {
		t.Fatalf("created players = %+v, want only original host", snapshot.Players)
	}

	response := client.get("/rooms/" + strings.ToLower(code))
	assertStatus(t, response, http.StatusOK)
	assertNoStore(t, response)
	for _, fragment := range []string{`data-room-code="` + code + `"`, `data-status="lobby"`, `id="room-state"`} {
		if !strings.Contains(response.Body.String(), fragment) {
			t.Errorf("room response missing %q", fragment)
		}
	}
	escapedName := html.EscapeString(player.Name)
	for _, fragment := range []string{
		`<span class="viewer-name">` + escapedName + `</span>`,
		`<span class="crew-name">` + escapedName + `</span>`,
		`<h2 id="directive-heading">` + html.EscapeString(string(player.Role)) + `</h2>`,
		`title="` + escapedName + `"`,
		`aria-label="Player P1, ` + escapedName + `, at `,
		fmt.Sprintf(">HEALTH %d</span>", player.Health),
		fmt.Sprintf(">STAMINA %d</span>", player.Stamina),
	} {
		if !strings.Contains(response.Body.String(), fragment) {
			t.Errorf("room response missing generated character fragment %q", fragment)
		}
	}
}

func TestJoinReconnectRevokesOldSessionAndProtectsRoomState(t *testing.T) {
	app := newRoomTestApp()
	host := app.client()
	code, _ := createRoom(t, host, "Host")

	guest := app.client()
	guest.get("/")
	joined := guest.post("/rooms/join", url.Values{
		"code":     {strings.ToLower(code)},
		"username": {"  Guest  "},
	})
	assertStatus(t, joined, http.StatusSeeOther)
	assertNoStore(t, joined)
	if got := joined.Header().Get("Location"); got != "/rooms/"+code {
		t.Fatalf("join Location = %q, want canonical room URL", got)
	}
	joinedSnapshot := mustSnapshot(t, app, code, guest.sessionID(t))
	guestPlayer := participantByUsername(t, joinedSnapshot, "Guest")
	if joinedSnapshot.Viewer.ID != guestPlayer.ID || joinedSnapshot.Viewer.Username != "Guest" {
		t.Fatalf("join viewer = %+v, want normalized guest identity", joinedSnapshot.Viewer)
	}
	if guestPlayer.Player.Name == "" || guestPlayer.Player.Role == "" {
		t.Fatalf("joined guest character = %+v, want generated name and directive", guestPlayer.Player)
	}
	if guestPlayer.Player.Health < 83 || guestPlayer.Player.Health > 100 || guestPlayer.Player.Stamina < 50 || guestPlayer.Player.Stamina > 100 || guestPlayer.Player.Strength < 0.80 || guestPlayer.Player.Strength > 1.00 || !guestPlayer.Player.Alive {
		t.Fatalf("joined guest stats are outside generated ranges: %+v", guestPlayer.Player)
	}
	guestPage := guest.get("/rooms/" + code)
	assertStatus(t, guestPage, http.StatusOK)
	guestState := guest.get("/rooms/" + code + "/state")
	assertStatus(t, guestState, http.StatusOK)
	escapedGuestName := html.EscapeString(guestPlayer.Player.Name)
	for _, rendered := range []struct {
		label string
		body  string
	}{
		{label: "guest page", body: guestPage.Body.String()},
		{label: "guest state", body: guestState.Body.String()},
	} {
		for _, fragment := range []string{
			`<span class="viewer-name">` + escapedGuestName + `</span>`,
			`<span class="crew-name">` + escapedGuestName + `</span>`,
		} {
			if !strings.Contains(rendered.body, fragment) {
				t.Errorf("%s missing generated guest character fragment %q", rendered.label, fragment)
			}
		}
	}

	unconnected := app.client()
	unconnected.get("/")
	page := unconnected.get("/rooms/" + code)
	assertStatus(t, page, http.StatusOK)
	assertNoStore(t, page)
	if !strings.Contains(page.Body.String(), `action="/rooms/`+code+`/join"`) {
		t.Fatal("unconnected room GET did not show room join form")
	}
	if strings.Contains(page.Body.String(), `id="room-state"`) || strings.Contains(page.Body.String(), `class="map-grid"`) {
		t.Fatal("unconnected room GET exposed game-state markup")
	}

	oldSessionID := guest.sessionID(t)
	reconnected := app.client()
	reconnected.get("/")
	reconnect := reconnected.post("/rooms/"+strings.ToLower(code)+"/join", url.Values{"username": {" gUeSt "}})
	assertStatus(t, reconnect, http.StatusSeeOther)
	assertNoStore(t, reconnect)
	if reconnect.Header().Get("Location") != "/rooms/"+code {
		t.Fatalf("reconnect Location = %q, want canonical room URL", reconnect.Header().Get("Location"))
	}
	reconnectedSnapshot := mustSnapshot(t, app, code, reconnected.sessionID(t))
	reconnectedPlayer := participantByUsername(t, reconnectedSnapshot, "Guest")
	if reconnectedPlayer.ID != guestPlayer.ID {
		t.Fatalf("reconnected player ID = %q, want preserved ID %q", reconnectedPlayer.ID, guestPlayer.ID)
	}
	if reconnectedPlayer.Player != guestPlayer.Player {
		t.Fatalf("reconnected player = %+v, want preserved first-join character %+v", reconnectedPlayer.Player, guestPlayer.Player)
	}
	if len(reconnectedSnapshot.Players) != 2 {
		t.Fatalf("player count after reconnect = %d, want 2", len(reconnectedSnapshot.Players))
	}
	reconnectedPage := reconnected.get("/rooms/" + code)
	assertStatus(t, reconnectedPage, http.StatusOK)
	for _, fragment := range []string{
		`<span class="viewer-name">` + escapedGuestName + `</span>`,
		`<span class="crew-name">` + escapedGuestName + `</span>`,
		`<h2 id="directive-heading">` + html.EscapeString(string(guestPlayer.Player.Role)) + `</h2>`,
	} {
		if !strings.Contains(reconnectedPage.Body.String(), fragment) {
			t.Errorf("reconnected room page missing preserved character fragment %q", fragment)
		}
	}
	if _, err := app.handler.Matches.Snapshot(code, oldSessionID); !errors.Is(err, match.ErrNotMember) {
		t.Fatalf("snapshot for replaced session error = %v, want ErrNotMember", err)
	}

	staleState := guest.get("/rooms/" + code + "/state")
	assertStatus(t, staleState, http.StatusUnauthorized)
	assertNoStore(t, staleState)
	if strings.Contains(staleState.Body.String(), `id="room-state"`) || strings.Contains(staleState.Body.String(), `class="map-grid"`) || strings.Contains(staleState.Body.String(), `data-player-id=`) {
		t.Fatal("stale session state response leaked room game state")
	}
	staleMutation := guest.post("/rooms/"+code+"/actions", url.Values{"direction": {"north"}})
	assertStatus(t, staleMutation, http.StatusUnauthorized)
	assertNoStore(t, staleMutation)
	if strings.Contains(staleMutation.Body.String(), `id="room-state"`) || strings.Contains(staleMutation.Body.String(), `data-player-id=`) {
		t.Fatal("stale session mutation response leaked player state")
	}

	hxState := guest.request(http.MethodGet, "/rooms/"+code+"/state", nil, "", true)
	assertStatus(t, hxState, http.StatusUnauthorized)
	assertNoStore(t, hxState)
	if got := hxState.Header().Get("HX-Redirect"); got != "/rooms/"+code {
		t.Errorf("unauthorized HX-Redirect = %q, want room URL", got)
	}
	if hxState.Body.Len() != 0 {
		t.Errorf("unauthorized HTMX state body = %q, want empty body", hxState.Body.String())
	}
}

func TestHostAuthorizationAndReconnectAcrossGameStates(t *testing.T) {
	app := newRoomTestApp()
	host := app.client()
	code, lobbySnapshot := createMovableRoom(t, app, host, "Host")
	if possibleDirection(lobbySnapshot) == "" {
		t.Fatal("movable-room helper returned room without a traversable start move")
	}
	member := app.client()
	member.get("/")
	join := member.post("/rooms/join", url.Values{"code": {code}, "username": {"Member"}})
	assertStatus(t, join, http.StatusSeeOther)
	assertNoStore(t, join)
	memberID := member.sessionID(t)
	memberSnapshot := mustSnapshot(t, app, code, memberID)
	memberPlayerID := memberSnapshot.Viewer.ID

	startDenied := member.post("/rooms/"+code+"/start", url.Values{})
	assertStatus(t, startDenied, http.StatusForbidden)
	assertNoStore(t, startDenied)
	if !strings.Contains(startDenied.Body.String(), "only the host") {
		t.Errorf("nonhost start response missing authorization explanation: %s", startDenied.Body.String())
	}

	start := host.post("/rooms/"+code+"/start", url.Values{})
	assertStatus(t, start, http.StatusSeeOther)
	assertNoStore(t, start)
	active := mustSnapshot(t, app, code, host.sessionID(t))
	if active.Status != match.Active {
		t.Fatalf("status after host start = %q, want active", active.Status)
	}

	finishDenied := member.post("/rooms/"+code+"/finish", url.Values{"playerID": {active.HostPlayerID}})
	assertStatus(t, finishDenied, http.StatusForbidden)
	assertNoStore(t, finishDenied)
	if !strings.Contains(finishDenied.Body.String(), "only the host") {
		t.Errorf("nonhost finish response missing authorization explanation: %s", finishDenied.Body.String())
	}

	activeMember := mustSnapshot(t, app, code, memberID)
	direction := possibleDirection(activeMember)
	if direction == "" {
		t.Fatal("generated start room has no possible adjacent movement")
	}
	moveResponse := member.post("/rooms/"+code+"/actions", url.Values{
		"direction": {direction},
		"playerID":  {active.HostPlayerID},
	})
	assertStatus(t, moveResponse, http.StatusSeeOther)
	assertNoStore(t, moveResponse)
	afterMove := mustSnapshot(t, app, code, memberID)
	hostAfterMove := mustSnapshot(t, app, code, host.sessionID(t))
	if afterMove.Viewer.ID != memberPlayerID {
		t.Fatalf("action viewer ID = %q, want session-bound member %q", afterMove.Viewer.ID, memberPlayerID)
	}
	if afterMove.Viewer.Player.Position == activeMember.Viewer.Player.Position {
		t.Fatalf("member action %q did not move session-bound player", direction)
	}
	if hostAfterMove.Viewer.Player.Position != active.Viewer.Player.Position {
		t.Fatalf("extra playerID changed host position from %+v to %+v", active.Viewer.Player.Position, hostAfterMove.Viewer.Player.Position)
	}

	newJoiner := app.client()
	newJoiner.get("/")
	joinActive := newJoiner.post("/rooms/"+strings.ToLower(code)+"/join", url.Values{"username": {"NewPlayer"}})
	assertStatus(t, joinActive, http.StatusConflict)
	assertNoStore(t, joinActive)
	if !strings.Contains(joinActive.Body.String(), "already started") {
		t.Errorf("active-room join error missing explanation: %s", joinActive.Body.String())
	}

	activeReconnect := app.client()
	activeReconnect.get("/")
	activeReconnectResponse := activeReconnect.post("/rooms/"+code+"/join", url.Values{"username": {" MEMBER "}})
	assertStatus(t, activeReconnectResponse, http.StatusSeeOther)
	assertNoStore(t, activeReconnectResponse)
	activeReconnectSnapshot := mustSnapshot(t, app, code, activeReconnect.sessionID(t))
	if activeReconnectSnapshot.Viewer.ID != memberPlayerID {
		t.Fatalf("active reconnect player ID = %q, want %q", activeReconnectSnapshot.Viewer.ID, memberPlayerID)
	}
	if activeReconnectSnapshot.Viewer.Player != afterMove.Viewer.Player {
		t.Fatalf("active reconnect player = %+v, want progressed player state %+v", activeReconnectSnapshot.Viewer.Player, afterMove.Viewer.Player)
	}
	staleMemberAction := member.post("/rooms/"+code+"/finish", url.Values{"playerID": {active.HostPlayerID}})
	assertStatus(t, staleMemberAction, http.StatusUnauthorized)
	assertNoStore(t, staleMemberAction)

	finish := host.post("/rooms/"+code+"/finish", url.Values{})
	assertStatus(t, finish, http.StatusSeeOther)
	assertNoStore(t, finish)
	finished := mustSnapshot(t, app, code, host.sessionID(t))
	if finished.Status != match.Finished {
		t.Fatalf("status after host finish = %q, want finished", finished.Status)
	}
	finishedReconnect := app.client()
	finishedReconnect.get("/")
	finishedJoin := finishedReconnect.post("/rooms/join", url.Values{"code": {strings.ToLower(code)}, "username": {"member"}})
	assertStatus(t, finishedJoin, http.StatusSeeOther)
	assertNoStore(t, finishedJoin)
	finishedSnapshot := mustSnapshot(t, app, code, finishedReconnect.sessionID(t))
	if finishedSnapshot.Status != match.Finished || finishedSnapshot.Viewer.ID != memberPlayerID {
		t.Fatalf("finished reconnect snapshot = status %q viewer %q, want finished and %q", finishedSnapshot.Status, finishedSnapshot.Viewer.ID, memberPlayerID)
	}
	if finishedSnapshot.Viewer.Player != afterMove.Viewer.Player {
		t.Fatalf("finished reconnect player = %+v, want progressed player state %+v", finishedSnapshot.Viewer.Player, afterMove.Viewer.Player)
	}
}

func possibleDirection(snapshot match.Snapshot) string {
	for _, candidate := range []struct {
		name     string
		possible bool
	}{
		{name: "north", possible: snapshot.Moves.North.Possible},
		{name: "east", possible: snapshot.Moves.East.Possible},
		{name: "south", possible: snapshot.Moves.South.Possible},
		{name: "west", possible: snapshot.Moves.West.Possible},
	} {
		if candidate.possible {
			return candidate.name
		}
	}
	return ""
}

func TestFormErrorsUnknownRoutesAndMethodSemantics(t *testing.T) {
	app := newRoomTestApp()
	client := app.client()
	client.get("/")

	emptyForm := client.post("/rooms", url.Values{})
	assertStatus(t, emptyForm, http.StatusUnprocessableEntity)
	if !strings.Contains(emptyForm.Body.String(), `role="alert"`) || !strings.Contains(emptyForm.Body.String(), "username") {
		t.Fatalf("empty form response lacks visible explanation: %s", emptyForm.Body.String())
	}

	invalidUsername := client.post("/rooms", url.Values{"username": {" \t "}})
	assertStatus(t, invalidUsername, http.StatusUnprocessableEntity)
	if !strings.Contains(invalidUsername.Body.String(), `role="alert"`) || !strings.Contains(invalidUsername.Body.String(), "username") {
		t.Fatalf("invalid username response lacks visible explanation: %s", invalidUsername.Body.String())
	}

	malformed := client.request(http.MethodPost, "/rooms", nil, "username=%ZZ", false)
	assertStatus(t, malformed, http.StatusBadRequest)
	if !strings.Contains(malformed.Body.String(), "Unable to read form submission") {
		t.Errorf("malformed form response lacks explanation: %s", malformed.Body.String())
	}

	unknownRoom := client.get("/rooms/NOTREAL")
	assertStatus(t, unknownRoom, http.StatusNotFound)
	assertNoStore(t, unknownRoom)
	unknownJoin := client.post("/rooms/join", url.Values{"code": {"NOTREAL"}, "username": {"Player"}})
	assertStatus(t, unknownJoin, http.StatusNotFound)
	if !strings.Contains(unknownJoin.Body.String(), "room") {
		t.Errorf("unknown-room join response lacks explanation: %s", unknownJoin.Body.String())
	}
	unknownPath := client.get("/no-such-route")
	assertStatus(t, unknownPath, http.StatusNotFound)

	code, _ := createRoom(t, client, "Host")
	wrongMethod := client.get("/rooms/" + code + "/start")
	assertStatus(t, wrongMethod, http.StatusMethodNotAllowed)
	assertNoStore(t, wrongMethod)

	oversized := client.request(http.MethodPost, "/rooms", nil, "username="+strings.Repeat("x", 17*1024), false)
	assertStatus(t, oversized, http.StatusRequestEntityTooLarge)
	if !strings.Contains(oversized.Body.String(), "too large") {
		t.Errorf("oversized form response lacks explanation: %s", oversized.Body.String())
	}
	if location := oversized.Header().Get("Location"); location != "" {
		t.Errorf("oversized form unexpectedly redirected to %q", location)
	}
}

func TestHTMXFragmentsAndMutationResponseSemantics(t *testing.T) {
	app := newRoomTestApp()
	host := app.client()
	code, lobbySnapshot := createMovableRoom(t, app, host, "Host")
	if possibleDirection(lobbySnapshot) == "" {
		t.Fatal("movable-room helper returned room without a traversable start move")
	}
	member := app.client()
	member.get("/")
	join := member.post("/rooms/"+code+"/join", url.Values{"username": {"Member"}})
	assertStatus(t, join, http.StatusSeeOther)
	assertNoStore(t, join)

	startError := member.postHTMX("/rooms/"+code+"/start", url.Values{})
	assertStatus(t, startError, http.StatusForbidden)
	assertNoStore(t, startError)
	assertRoomStateFragment(t, startError.Body.String(), "only the host")

	actionError := host.postHTMX("/rooms/"+code+"/actions", url.Values{"direction": {"north"}})
	assertStatus(t, actionError, http.StatusConflict)
	assertNoStore(t, actionError)
	assertRoomStateFragment(t, actionError.Body.String(), "not active")

	startHTMX := host.postHTMX("/rooms/"+code+"/start", url.Values{})
	assertStatus(t, startHTMX, http.StatusOK)
	assertNoStore(t, startHTMX)
	if got := startHTMX.Header().Get("HX-Redirect"); got != "" {
		t.Errorf("accepted HTMX start HX-Redirect = %q, want none", got)
	}
	assertRoomStateFragment(t, startHTMX.Body.String(), "data-status=\"active\"")

	started := mustSnapshot(t, app, code, host.sessionID(t))
	beforeInvalidMove := started.Viewer.Player.Position
	beforeRevision := started.Revision
	invalidMove := host.post("/rooms/"+code+"/actions", url.Values{"direction": {"sideways"}})
	assertStatus(t, invalidMove, http.StatusUnprocessableEntity)
	assertNoStore(t, invalidMove)
	if !strings.Contains(invalidMove.Body.String(), "invalid or blocked move") {
		t.Errorf("invalid direction response lacks explanation: %s", invalidMove.Body.String())
	}
	afterInvalidMove := mustSnapshot(t, app, code, host.sessionID(t))
	if afterInvalidMove.Revision != beforeRevision || afterInvalidMove.Viewer.Player.Position != beforeInvalidMove {
		t.Fatalf("invalid move mutated snapshot: revision %d -> %d, position %+v -> %+v", beforeRevision, afterInvalidMove.Revision, beforeInvalidMove, afterInvalidMove.Viewer.Player.Position)
	}

	ordinaryStart := host.post("/rooms/"+code+"/start", url.Values{})
	assertStatus(t, ordinaryStart, http.StatusSeeOther)
	assertNoStore(t, ordinaryStart)
	direction := possibleDirection(afterInvalidMove)
	if direction == "" {
		t.Fatal("generated start room has no possible adjacent movement")
	}
	ordinaryAction := host.post("/rooms/"+code+"/actions", url.Values{"direction": {direction}})
	assertStatus(t, ordinaryAction, http.StatusSeeOther)
	assertNoStore(t, ordinaryAction)
}

func assertRoomStateFragment(t *testing.T, body, expected string) {
	t.Helper()
	if !strings.Contains(body, `id="room-state"`) {
		t.Fatalf("HTMX error response missing #room-state fragment: %s", body)
	}
	if !strings.Contains(body, expected) {
		t.Fatalf("HTMX response missing expected status/message %q: %s", expected, body)
	}
	if strings.Contains(body, "<!DOCTYPE html>") || strings.Contains(body, "<html") {
		t.Fatalf("HTMX response contained full document instead of fragment: %s", body)
	}
}

func TestLeaveRevokesMembershipAndClosesEmptyRoom(t *testing.T) {
	app := newRoomTestApp()
	host := app.client()
	code, _ := createRoom(t, host, "Host")
	member := app.client()
	member.get("/")
	joined := member.post("/rooms/join", url.Values{"code": {code}, "username": {"Member"}})
	assertStatus(t, joined, http.StatusSeeOther)
	assertNoStore(t, joined)

	leave := member.post("/rooms/"+code+"/leave", url.Values{})
	assertStatus(t, leave, http.StatusSeeOther)
	assertNoStore(t, leave)
	if got := leave.Header().Get("Location"); got != "/" {
		t.Errorf("leave Location = %q, want home", got)
	}
	oldState := member.get("/rooms/" + code + "/state")
	assertStatus(t, oldState, http.StatusUnauthorized)
	assertNoStore(t, oldState)
	if strings.Contains(oldState.Body.String(), `id="room-state"`) || strings.Contains(oldState.Body.String(), `data-player-id=`) {
		t.Fatal("left member state response exposed room/player state")
	}
	if got := len(mustSnapshot(t, app, code, host.sessionID(t)).Players); got != 1 {
		t.Fatalf("remaining player count after member leave = %d, want 1", got)
	}

	lastLeave := host.post("/rooms/"+code+"/leave", url.Values{})
	assertStatus(t, lastLeave, http.StatusSeeOther)
	assertNoStore(t, lastLeave)
	if _, err := app.handler.Matches.Snapshot(code, host.sessionID(t)); !errors.Is(err, match.ErrRoomNotFound) {
		t.Fatalf("snapshot after final member leave error = %v, want ErrRoomNotFound", err)
	}
	closed := host.get("/rooms/" + code)
	assertStatus(t, closed, http.StatusNotFound)
	assertNoStore(t, closed)
}

func TestRoomResponsesExposeRosterFields(t *testing.T) {
	app := newRoomTestApp()
	client := app.client()
	code, _ := createRoom(t, client, "Player")
	response := client.get("/rooms/" + code)
	assertStatus(t, response, http.StatusOK)
	snapshot := mustSnapshot(t, app, code, client.sessionID(t))
	player := snapshot.Viewer
	for _, attribute := range []string{
		fmt.Sprintf(`data-player-id="%s"`, player.ID),
		fmt.Sprintf(`data-x="%d"`, player.Player.Position.X),
		fmt.Sprintf(`data-y="%d"`, player.Player.Position.Y),
		fmt.Sprintf(`data-health="%d"`, player.Player.Health),
	} {
		if !strings.Contains(response.Body.String(), attribute) {
			t.Errorf("room roster missing %q", attribute)
		}
	}
}
