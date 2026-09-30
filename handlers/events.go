package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"seesharpsi/web_roguelike/match"
	"seesharpsi/web_roguelike/templ"
)

const eventHeartbeatInterval = 15 * time.Second

func (h *Handler) RoomEvents(w http.ResponseWriter, r *http.Request) {
	if h.Manager == nil || h.Matches == nil {
		http.Error(w, "Room event service unavailable", http.StatusInternalServerError)
		return
	}

	cookie, err := r.Cookie("session_id")
	if err != nil || cookie.Value == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	sessionID := cookie.Value
	if h.Manager.GetSession(sessionID) == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	code := match.NormalizeCode(r.PathValue("code"))
	snapshot, changed, err := h.Matches.Watch(code, sessionID)
	if err != nil {
		h.writeInitialWatchError(w, err)
		return
	}

	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream;charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	if _, ok := w.(http.Flusher); !ok {
		// Probe wrappers that expose flushing through ResponseController.Unwrap.
		if err := controller.Flush(); err != nil {
			http.Error(w, "Streaming is not supported", http.StatusInternalServerError)
			return
		}
	} else {
		w.WriteHeader(http.StatusOK)
	}

	lastSent := snapshot.Revision
	if !h.writeRoomEvent(w, r.Context(), controller, snapshot) {
		return
	}

	heartbeat := time.NewTicker(eventHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-changed:
			if h.Manager.GetSession(sessionID) == nil {
				h.writeRoomRedirect(w, controller, code)
				return
			}

			snapshot, nextChange, err := h.Matches.Watch(code, sessionID)
			if err != nil {
				if shouldRedirectRoomEvents(err) {
					h.writeRoomRedirect(w, controller, code)
				}
				return
			}
			changed = nextChange
			if snapshot.Revision <= lastSent {
				continue
			}
			if !h.writeRoomEvent(w, r.Context(), controller, snapshot) {
				return
			}
			lastSent = snapshot.Revision
		case <-heartbeat.C:
			if h.Manager.GetSession(sessionID) == nil {
				h.writeRoomRedirect(w, controller, code)
				return
			}
			if !writeSSEFrame(w, controller, ": keepalive\n\n") {
				return
			}
		}
	}
}

func (h *Handler) writeInitialWatchError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, match.ErrNotMember):
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	case errors.Is(err, match.ErrRoomNotFound), errors.Is(err, match.ErrClosed):
		http.Error(w, "Room not found", http.StatusNotFound)
	default:
		http.Error(w, "Unable to load room events", http.StatusInternalServerError)
	}
}

func (h *Handler) writeRoomEvent(w http.ResponseWriter, ctx context.Context, controller *http.ResponseController, snapshot match.Snapshot) bool {
	var body bytes.Buffer
	if err := templ.RoomState(snapshot, "").Render(ctx, &body); err != nil {
		return false
	}
	frame := encodeSSEFrame("", strconv.FormatUint(snapshot.Revision, 10), body.String())
	return writeSSEFrame(w, controller, frame)
}

func (h *Handler) writeRoomRedirect(w http.ResponseWriter, controller *http.ResponseController, code string) {
	writeSSEFrame(w, controller, encodeSSEFrame("room-redirect", "", roomURL(code)))
}

func shouldRedirectRoomEvents(err error) bool {
	return errors.Is(err, match.ErrNotMember) || errors.Is(err, match.ErrClosed) || errors.Is(err, match.ErrRoomNotFound)
}

func writeSSEFrame(w http.ResponseWriter, controller *http.ResponseController, frame string) bool {
	_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
	defer controller.SetWriteDeadline(time.Time{})

	n, err := io.WriteString(w, frame)
	if err != nil || n != len(frame) {
		return false
	}
	return controller.Flush() == nil
}

func encodeSSEFrame(event, id, data string) string {
	var frame strings.Builder
	if event != "" {
		fmt.Fprintf(&frame, "event: %s\n", event)
	}
	if id != "" {
		fmt.Fprintf(&frame, "id: %s\n", id)
	}
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")
	for _, line := range strings.Split(data, "\n") {
		frame.WriteString("data: ")
		frame.WriteString(line)
		frame.WriteByte('\n')
	}
	frame.WriteByte('\n')
	return frame.String()
}
