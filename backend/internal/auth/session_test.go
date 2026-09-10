package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestSessionRejectsWeakInputsAndAlteredHeaders(t *testing.T) {
	if _, err := SignSession("short", "user", "", ""); err == nil {
		t.Fatal("weak session secret accepted")
	}
	token, err := SignSession("12345678901234567890123456789012", "user", "user@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	parts[0] = "eyJhbGciOiJub25lIn0"
	if _, err := VerifySession("12345678901234567890123456789012", strings.Join(parts, ".")); err == nil {
		t.Fatal("altered JWT header accepted")
	}
	if user, err := VerifySession("12345678901234567890123456789012", token); err != nil || user.GoogleID != "user" {
		t.Fatalf("valid session failed: user=%+v err=%v", user, err)
	}
}

func TestLoginCanRequestGoogleAccountChooser(t *testing.T) {
	handler := &Handler{oauth: &oauth2.Config{
		ClientID: "client",
		Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.example/auth"},
	}}
	recorder := httptest.NewRecorder()
	handler.Login(recorder, httptest.NewRequest(http.MethodGet, "/auth/login/google?select=1", nil))

	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusFound || location.Query().Get("prompt") != "select_account" || location.Query().Get("state") == "" {
		t.Fatalf("unexpected OAuth redirect: status=%d location=%s", recorder.Code, location)
	}
}
