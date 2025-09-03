package handlers

import (
	"context"
	"net/http"

	"seesharpsi/web_roguelike/session"
	"seesharpsi/web_roguelike/templ"
)

type Handler struct {
	Manager *session.Manager
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	print("\nGot / request")
	// replace _ with sess if you want to use a session
	_, cookie := h.Manager.GetOrCreateSession(r)
	http.SetCookie(w, &cookie)

	templ.Index().Render(context.Background(), w)
}

func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	print("\nGot /test request")
	// replace _ with sess if you want to use a session
	_, cookie := h.Manager.GetOrCreateSession(r)
	http.SetCookie(w, &cookie)

	templ.Test().Render(context.Background(), w)
}

func (h *Handler) Map(w http.ResponseWriter, r *http.Request) {
	print("\nGot /map request")
	sess, cookie := h.Manager.GetOrCreateSession(r)
	http.SetCookie(w, &cookie)

	templ.Map(sess.Map, sess.Player).Render(context.Background(), w)
}

func (h *Handler) Move(w http.ResponseWriter, r *http.Request) {
	print("\nGot /move request")
	sess, cookie := h.Manager.GetOrCreateSession(r)
	http.SetCookie(w, &cookie)

	// Get direction from query parameter
	direction := r.URL.Query().Get("direction")
	if direction == "" {
		http.Error(w, "Missing direction parameter", http.StatusBadRequest)
		return
	}

	// Attempt to move the player
	sess.Map.MovePlayer(&sess.Player, direction)

	// Return the full map (including health display) for HTMX to replace
	w.Header().Set("Content-Type", "text/html")
	templ.Map(sess.Map, sess.Player).Render(context.Background(), w)
}
