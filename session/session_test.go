package session_test

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"seesharpsi/web_roguelike/session"
)

func TestGetOrCreateSessionReusesCookieIdentity(t *testing.T) {
	manager := session.NewManager()
	request := httptest.NewRequest("GET", "http://example.test/", nil)
	created, cookie, err := manager.GetOrCreateSession(request)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || cookie.Value != created.ID {
		t.Fatalf("new session ID %q does not match cookie value %q", created.ID, cookie.Value)
	}
	assertSessionCookie(t, cookie, false)

	request = httptest.NewRequest("GET", "http://example.test/next", nil)
	request.AddCookie(&cookie)
	reused, refreshedCookie, err := manager.GetOrCreateSession(request)
	if err != nil {
		t.Fatal(err)
	}
	if reused.ID != created.ID || refreshedCookie.Value != created.ID {
		t.Fatalf("cookie did not reuse session identity: session=%q cookie=%q want=%q", reused.ID, refreshedCookie.Value, created.ID)
	}
	assertSessionCookie(t, refreshedCookie, false)
}

func TestSessionCookieUsesSecureAndExpectedAttributes(t *testing.T) {
	manager := session.NewManager()
	request := httptest.NewRequest("GET", "https://example.test/", nil)
	request.TLS = &tls.ConnectionState{}
	created, cookie, err := manager.GetOrCreateSession(request)
	if err != nil {
		t.Fatal(err)
	}
	if cookie.Value != created.ID {
		t.Fatalf("cookie value = %q, want session ID %q", cookie.Value, created.ID)
	}
	assertSessionCookie(t, cookie, true)
}

func TestExpiredCookieCreatesIndependentSession(t *testing.T) {
	manager := session.NewManager()
	initial, initialCookie, err := manager.GetOrCreateSession(httptest.NewRequest("GET", "http://example.test/", nil))
	if err != nil {
		t.Fatal(err)
	}

	manager.Cleanup(time.Now().Add(48*time.Hour), time.Hour)
	if got := manager.GetSession(initial.ID); got != nil {
		t.Fatalf("expired session %q still exists", initial.ID)
	}

	staleCookieRequest := httptest.NewRequest("GET", "http://example.test/", nil)
	staleCookieRequest.AddCookie(&initialCookie)
	replacement, replacementCookie, err := manager.GetOrCreateSession(staleCookieRequest)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.ID == initial.ID {
		t.Fatalf("expired cookie reused removed session ID %q", initial.ID)
	}
	if replacementCookie.Value != replacement.ID {
		t.Fatalf("replacement cookie value = %q, want %q", replacementCookie.Value, replacement.ID)
	}
	if manager.GetSession(replacement.ID) == nil {
		t.Fatal("replacement session was not stored")
	}
}

func TestReturnedSessionMutationDoesNotChangeStoredSession(t *testing.T) {
	manager := session.NewManager()
	id, err := manager.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	returned := manager.GetSession(id)
	if returned == nil {
		t.Fatal("created session could not be retrieved")
	}

	returned.ID = "mutated-copy"
	returned.LastAccessed = time.Unix(1, 0)
	stored := manager.GetSession(id)
	if stored == nil {
		t.Fatal("mutating returned value removed stored session")
	}
	if stored.ID != id {
		t.Fatalf("stored session ID changed to %q after copy mutation", stored.ID)
	}
	if stored.LastAccessed.Equal(time.Unix(1, 0)) {
		t.Fatal("stored LastAccessed changed after returned copy mutation")
	}
}

func TestConcurrentSessionReadsAreSafe(t *testing.T) {
	manager := session.NewManager()
	id, err := manager.CreateSession()
	if err != nil {
		t.Fatal(err)
	}

	const workersCount = 32
	const readsPerWorker = 100
	start := make(chan struct{})
	errors := make(chan error, workersCount)
	var workers sync.WaitGroup
	for worker := 0; worker < workersCount; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			<-start
			for read := 0; read < readsPerWorker; read++ {
				got := manager.GetSession(id)
				if got == nil {
					errors <- fmt.Errorf("worker %d read %d: session disappeared", worker, read)
					return
				}
				if got.ID != id {
					errors <- fmt.Errorf("worker %d read %d: session ID = %q, want %q", worker, read, got.ID, id)
					return
				}
			}
		}(worker)
	}
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}

func assertSessionCookie(t *testing.T, cookie http.Cookie, secure bool) {
	t.Helper()
	if cookie.Name != "session_id" {
		t.Errorf("cookie name = %q, want session_id", cookie.Name)
	}
	if cookie.Path != "/" {
		t.Errorf("cookie path = %q, want /", cookie.Path)
	}
	if !cookie.HttpOnly {
		t.Error("session cookie is not HttpOnly")
	}
	if cookie.Secure != secure {
		t.Errorf("cookie Secure = %t, want %t", cookie.Secure, secure)
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie SameSite = %v, want SameSiteLaxMode", cookie.SameSite)
	}
	if cookie.MaxAge != 24*60*60 {
		t.Errorf("cookie MaxAge = %d, want 24 hours", cookie.MaxAge)
	}
	if cookie.Expires.IsZero() || time.Until(cookie.Expires) <= 0 {
		t.Errorf("cookie expiry %v is not in the future", cookie.Expires)
	}
}
