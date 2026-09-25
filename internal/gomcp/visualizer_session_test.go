package gomcp

import (
	"bytes"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVisualizerSessionSurvivesReloadAndOnlyReadsVisualizer(t *testing.T) {
	server, _ := testServer(t, true)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	call := func(method, path, bearer, origin, body string) *http.Response {
		t.Helper()
		request, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if bearer != "" {
			request.Header.Set("Authorization", "Bearer "+bearer)
		}
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		contents, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		response.Body = io.NopCloser(bytes.NewReader(contents))
		return response
	}
	initial := call(http.MethodGet, "/visualizer/api/session", "", "", "")
	initialBody, _ := io.ReadAll(initial.Body)
	if initial.StatusCode != http.StatusOK || !bytes.Contains(initialBody, []byte("\"connected\":false")) {
		t.Fatalf("initial session: status=%d body=%s", initial.StatusCode, initialBody)
	}
	if got := call(http.MethodPost, "/visualizer/api/session", "wrong-token", server.URL, "").StatusCode; got != http.StatusUnauthorized {
		t.Fatalf("wrong token created session: %d", got)
	}
	login := call(http.MethodPost, "/visualizer/api/session", testToken, server.URL, "")
	if login.StatusCode != http.StatusNoContent || login.Header.Get("Cache-Control") != "no-store" || len(login.Cookies()) != 1 {
		t.Fatalf("login status=%d cookies=%d", login.StatusCode, len(login.Cookies()))
	}
	cookie := login.Cookies()[0]
	if cookie.Name != visualizerCookieName || cookie.Path != "/visualizer/api/" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Secure || cookie.MaxAge != int(visualizerSessionLifetime.Seconds()) {
		t.Fatalf("unsafe or short-lived visualizer cookie: %+v", cookie)
	}
	remembered := call(http.MethodGet, "/visualizer/api/session", "", "", "")
	rememberedBody, _ := io.ReadAll(remembered.Body)
	if remembered.StatusCode != http.StatusOK || !bytes.Contains(rememberedBody, []byte("\"connected\":true")) {
		t.Fatalf("remembered session: status=%d body=%s", remembered.StatusCode, rememberedBody)
	}
	if got := call(http.MethodPost, "/visualizer/api/context", "", server.URL, "{}").StatusCode; got != http.StatusOK {
		t.Fatalf("cookie could not read visualizer: %d", got)
	}
	if got := call(http.MethodPost, "/visualizer/api/context", "", "", "{}").StatusCode; got != http.StatusForbidden {
		t.Fatalf("cookie read without same Origin: %d", got)
	}
	if got := call(http.MethodPost, "/visualizer/api/context", "", "https://other.example", "{}").StatusCode; got != http.StatusForbidden {
		t.Fatalf("cross-origin cookie read: %d", got)
	}
	for _, path := range []string{"/healthz", "/mcp"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(cookie)
		response, err := http.DefaultClient.Do(func() *http.Request {
			actual, requestErr := http.NewRequest(http.MethodGet, server.URL+path, nil)
			if requestErr != nil {
				t.Fatal(requestErr)
			}
			actual.Header = request.Header
			return actual
		}())
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("visualizer cookie authenticated %s: %d", path, response.StatusCode)
		}
	}
	if got := call(http.MethodDelete, "/visualizer/api/session", "", server.URL, "").StatusCode; got != http.StatusNoContent {
		t.Fatalf("logout status %d", got)
	}
	loggedOut := call(http.MethodGet, "/visualizer/api/session", "", "", "")
	loggedOutBody, _ := io.ReadAll(loggedOut.Body)
	if loggedOut.StatusCode != http.StatusOK || !bytes.Contains(loggedOutBody, []byte("\"connected\":false")) {
		t.Fatalf("logout did not clear browser session: status=%d body=%s", loggedOut.StatusCode, loggedOutBody)
	}
}

func TestVisualizerSessionExpiresAndRejectsTampering(t *testing.T) {
	key := sha256.Sum256([]byte(testToken))
	now := time.Now()
	value, err := newVisualizerSession(key[:], now.Add(visualizerSessionLifetime))
	if err != nil {
		t.Fatal(err)
	}
	if !validVisualizerSession(value, key[:], now) {
		t.Fatal("new session rejected")
	}
	if validVisualizerSession(value, key[:], now.Add(visualizerSessionLifetime+time.Second)) {
		t.Fatal("expired session accepted")
	}
	wrongKey := sha256.Sum256([]byte("rotated token"))
	if validVisualizerSession(value, wrongKey[:], now) {
		t.Fatal("session survived token rotation")
	}
	parts := strings.Split(value, ".")
	first := byte('A')
	if parts[3][0] == first {
		first = 'B'
	}
	changedSignature := strings.Join(parts[:3], ".") + "." + string(first) + parts[3][1:]
	for _, tampered := range []string{"bad", strings.Replace(value, "v1.", "v2.", 1), changedSignature} {
		if validVisualizerSession(tampered, key[:], now) {
			t.Fatalf("tampered session accepted: %q", tampered)
		}
	}
}

func TestVisualizerSessionCookieIsSecureOnPrivateHTTPSHost(t *testing.T) {
	_, store := testServer(t, true)
	handler, err := NewHandler(Backend{Store: store, Visualizer: true, AllowedProxyHost: "private.example:8456"}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/visualizer/api/session", nil)
	request.Host = "private.example:8456"
	request.Header.Set("Origin", "https://private.example:8456")
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || len(response.Result().Cookies()) != 1 || !response.Result().Cookies()[0].Secure {
		t.Fatalf("private HTTPS cookie not Secure: status=%d cookies=%v", response.Code, response.Result().Cookies())
	}
}
