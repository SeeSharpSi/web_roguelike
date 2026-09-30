package handlers

import "testing"

func TestEncodeSSEFramePrefixesEveryDataLine(t *testing.T) {
	got := encodeSSEFrame("", "42", "first\r\nsecond\revent: injected")
	want := "id: 42\ndata: first\ndata: second\ndata: event: injected\n\n"
	if got != want {
		t.Fatalf("encoded SSE frame = %q, want %q", got, want)
	}
}

func TestEncodeSSEFrameNamedRedirect(t *testing.T) {
	got := encodeSSEFrame("room-redirect", "", "/rooms/ABCDE2")
	want := "event: room-redirect\ndata: /rooms/ABCDE2\n\n"
	if got != want {
		t.Fatalf("encoded SSE frame = %q, want %q", got, want)
	}
}
