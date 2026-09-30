package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"seesharpsi/web_roguelike/match"
	"seesharpsi/web_roguelike/session"
	"seesharpsi/web_roguelike/templ"
)

const maxFormBody = 16 << 10

type Handler struct {
	Manager *session.Manager
	Matches *match.Registry
}

type renderable interface {
	Render(context.Context, io.Writer) error
}

type mutation func(code, sessionID, direction string) (match.Snapshot, error)

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.noStore(w)
		mux.ServeHTTP(w, r)
	})
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.Index)
	mux.HandleFunc("GET /test", h.Test)
	mux.HandleFunc("GET /map", h.Map)
	mux.HandleFunc("POST /rooms", h.CreateRoom)
	mux.HandleFunc("POST /rooms/join", h.JoinRoom)
	mux.HandleFunc("POST /rooms/{code}/join", h.JoinRoomByCode)
	mux.HandleFunc("GET /rooms/{code}", h.Room)
	mux.HandleFunc("GET /rooms/{code}/state", h.RoomState)
	mux.HandleFunc("GET /rooms/{code}/events", h.RoomEvents)
	mux.HandleFunc("POST /rooms/{code}/start", h.Start)
	mux.HandleFunc("POST /rooms/{code}/actions", h.Move)
	mux.HandleFunc("POST /rooms/{code}/finish", h.Finish)
	mux.HandleFunc("POST /rooms/{code}/leave", h.Leave)
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.getSession(w, r); !ok {
		return
	}
	h.render(w, r, http.StatusOK, templ.Index("", "", ""))
}

func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.getSession(w, r); !ok {
		return
	}
	h.render(w, r, http.StatusOK, templ.Test())
}

func (h *Handler) Map(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code != "" {
		http.Redirect(w, r, "/rooms/"+url.PathEscape(code), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSession(w, r)
	if !ok {
		return
	}
	if err := parseForm(w, r); err != nil {
		h.render(w, r, formErrorStatus(err), templ.Index("", "", formErrorMessage(err)))
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	snapshot, err := h.Matches.Create(sess.ID, username)
	if err != nil {
		h.render(w, r, errorStatus(err), templ.Index(username, "", err.Error()))
		return
	}
	http.Redirect(w, r, roomURL(snapshot.Code), http.StatusSeeOther)
}

func (h *Handler) JoinRoom(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSession(w, r)
	if !ok {
		return
	}
	if err := parseForm(w, r); err != nil {
		h.render(w, r, formErrorStatus(err), templ.Index(r.FormValue("username"), r.FormValue("code"), formErrorMessage(err)))
		return
	}
	code := match.NormalizeCode(r.FormValue("code"))
	username := strings.TrimSpace(r.FormValue("username"))
	snapshot, err := h.Matches.Join(code, sess.ID, username)
	if err != nil {
		h.render(w, r, errorStatus(err), templ.Index(username, code, err.Error()))
		return
	}
	http.Redirect(w, r, roomURL(snapshot.Code), http.StatusSeeOther)
}

func (h *Handler) JoinRoomByCode(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSession(w, r)
	if !ok {
		return
	}
	code := match.NormalizeCode(r.PathValue("code"))
	if err := parseForm(w, r); err != nil {
		h.render(w, r, formErrorStatus(err), templ.JoinRoomPage(code, r.FormValue("username"), formErrorMessage(err)))
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	snapshot, err := h.Matches.Join(code, sess.ID, username)
	if err != nil {
		h.renderJoinError(w, r, code, username, err)
		return
	}
	http.Redirect(w, r, roomURL(snapshot.Code), http.StatusSeeOther)
}

func (h *Handler) Room(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSession(w, r)
	if !ok {
		return
	}
	code := match.NormalizeCode(r.PathValue("code"))
	snapshot, err := h.Matches.Snapshot(code, sess.ID)
	if err != nil {
		switch {
		case errors.Is(err, match.ErrNotMember):
			h.render(w, r, http.StatusOK, templ.JoinRoomPage(code, "", ""))
		case errors.Is(err, match.ErrRoomNotFound), errors.Is(err, match.ErrClosed):
			h.render(w, r, http.StatusNotFound, templ.MissingRoomPage(code))
		default:
			http.Error(w, "Unable to load room", http.StatusInternalServerError)
		}
		return
	}
	h.render(w, r, http.StatusOK, templ.RoomPage(snapshot, ""))
}

func (h *Handler) RoomState(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSession(w, r)
	if !ok {
		return
	}
	code := match.NormalizeCode(r.PathValue("code"))
	snapshot, err := h.Matches.Snapshot(code, sess.ID)
	if err != nil {
		switch {
		case errors.Is(err, match.ErrNotMember):
			if isHTMX(r) {
				w.Header().Set("HX-Redirect", roomURL(code))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h.render(w, r, http.StatusUnauthorized, templ.JoinRoomPage(code, "", "Your session is not connected to this room. Rejoin to continue."))
		case errors.Is(err, match.ErrRoomNotFound), errors.Is(err, match.ErrClosed):
			if isHTMX(r) {
				w.Header().Set("HX-Redirect", roomURL(code))
				w.WriteHeader(http.StatusNotFound)
				return
			}
			h.render(w, r, http.StatusNotFound, templ.MissingRoomPage(code))
		default:
			http.Error(w, "Unable to load room state", http.StatusInternalServerError)
		}
		return
	}
	h.render(w, r, http.StatusOK, templ.RoomState(snapshot, ""))
}

func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	h.mutate(w, r, func(code, sessionID, _ string) (match.Snapshot, error) {
		return h.Matches.Start(code, sessionID)
	})
}

func (h *Handler) Move(w http.ResponseWriter, r *http.Request) {
	h.mutate(w, r, func(code, sessionID, direction string) (match.Snapshot, error) {
		return h.Matches.Move(code, sessionID, direction)
	})
}

func (h *Handler) Finish(w http.ResponseWriter, r *http.Request) {
	h.mutate(w, r, func(code, sessionID, _ string) (match.Snapshot, error) {
		return h.Matches.Finish(code, sessionID)
	})
}

func (h *Handler) Leave(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.getSession(w, r)
	if !ok {
		return
	}
	if err := parseForm(w, r); err != nil {
		h.renderMutationError(w, r, sess.ID, formErrorStatus(err), formErrorMessage(err), nil)
		return
	}
	code := match.NormalizeCode(r.PathValue("code"))
	if err := h.Matches.Leave(code, sess.ID); err != nil {
		h.renderMutationError(w, r, sess.ID, errorStatus(err), err.Error(), err)
		return
	}
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) mutate(w http.ResponseWriter, r *http.Request, apply mutation) {
	sess, ok := h.getSession(w, r)
	if !ok {
		return
	}
	if err := parseForm(w, r); err != nil {
		h.renderMutationError(w, r, sess.ID, formErrorStatus(err), formErrorMessage(err), nil)
		return
	}
	code := match.NormalizeCode(r.PathValue("code"))
	snapshot, err := apply(code, sess.ID, strings.TrimSpace(r.FormValue("direction")))
	if err != nil {
		h.renderMutationError(w, r, sess.ID, errorStatus(err), err.Error(), err)
		return
	}
	if isHTMX(r) {
		h.render(w, r, http.StatusOK, templ.RoomState(snapshot, ""))
		return
	}
	http.Redirect(w, r, roomURL(snapshot.Code), http.StatusSeeOther)
}

func (h *Handler) renderMutationError(w http.ResponseWriter, r *http.Request, sessionID string, status int, message string, cause error) {
	code := match.NormalizeCode(r.PathValue("code"))
	if errors.Is(cause, match.ErrNotMember) {
		if isHTMX(r) {
			w.Header().Set("HX-Redirect", roomURL(code))
			w.WriteHeader(status)
			return
		}
		h.render(w, r, status, templ.JoinRoomPage(code, "", "Your session is not connected to this room. Rejoin to continue."))
		return
	}
	snapshot, err := h.Matches.Snapshot(code, sessionID)
	if err != nil {
		if errors.Is(err, match.ErrNotMember) {
			if isHTMX(r) {
				w.Header().Set("HX-Redirect", roomURL(code))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h.render(w, r, http.StatusUnauthorized, templ.JoinRoomPage(code, "", "Your session is not connected to this room. Rejoin to continue."))
			return
		}
		if errors.Is(err, match.ErrRoomNotFound) || errors.Is(err, match.ErrClosed) {
			if isHTMX(r) {
				w.Header().Set("HX-Redirect", roomURL(code))
				w.WriteHeader(http.StatusNotFound)
				return
			}
			h.render(w, r, http.StatusNotFound, templ.MissingRoomPage(code))
			return
		}
		http.Error(w, "Unable to load room state", http.StatusInternalServerError)
		return
	}
	if isHTMX(r) {
		h.render(w, r, status, templ.RoomState(snapshot, message))
		return
	}
	h.render(w, r, status, templ.RoomPage(snapshot, message))
}

func (h *Handler) renderJoinError(w http.ResponseWriter, r *http.Request, code, username string, err error) {
	status := errorStatus(err)
	if errors.Is(err, match.ErrRoomNotFound) || errors.Is(err, match.ErrClosed) {
		h.render(w, r, http.StatusNotFound, templ.JoinRoomPage(code, username, err.Error()))
		return
	}
	h.render(w, r, status, templ.JoinRoomPage(code, username, err.Error()))
}

func (h *Handler) getSession(w http.ResponseWriter, r *http.Request) (*session.Session, bool) {
	if h.Manager == nil {
		http.Error(w, "Session manager unavailable", http.StatusInternalServerError)
		return nil, false
	}
	sess, cookie, err := h.Manager.GetOrCreateSession(r)
	if err != nil {
		http.Error(w, "Unable to load session", http.StatusInternalServerError)
		return nil, false
	}
	http.SetCookie(w, &cookie)
	return sess, true
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, component renderable) {
	var output bytes.Buffer
	if err := component.Render(r.Context(), &output); err != nil {
		http.Error(w, "Unable to render page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := output.WriteTo(w); err != nil {
		return
	}
}

func (h *Handler) noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}

func parseForm(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBody)
	if err := r.ParseForm(); err != nil {
		return err
	}
	return nil
}

func formErrorStatus(err error) int {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func formErrorMessage(err error) string {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return "Form submission is too large."
	}
	return "Unable to read form submission. Check the fields and try again."
}

func errorStatus(err error) int {
	switch {
	case errors.Is(err, match.ErrRoomNotFound):
		return http.StatusNotFound
	case errors.Is(err, match.ErrNotMember):
		return http.StatusUnauthorized
	case errors.Is(err, match.ErrInvalidUsername), errors.Is(err, match.ErrInvalidMove):
		return http.StatusUnprocessableEntity
	case errors.Is(err, match.ErrNotHost):
		return http.StatusForbidden
	case errors.Is(err, match.ErrRoomFull), errors.Is(err, match.ErrGameStarted), errors.Is(err, match.ErrNotActive):
		return http.StatusConflict
	case errors.Is(err, match.ErrClosed):
		return http.StatusGone
	default:
		return http.StatusInternalServerError
	}
}

func roomURL(code string) string {
	return "/rooms/" + url.PathEscape(code)
}

func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
