package gomcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOwnerSignInOnlyUsesTrustedHostAndOrigin(t *testing.T) {
	_, store := testServer(t, true)
	handler, err := NewHandler(Backend{Store: store, Visualizer: true, AllowedProxyHost: "memory.example:443"}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		host, origin string
		want         int
	}{
		{"memory.example:443", "https://memory.example:443", 204},
		{"memory.example:443", "http://memory.example:443", 403},
		{"wrong.example:443", "https://wrong.example:443", 403},
		{"wrong.example:443", "http://wrong.example:443", 403},
	} {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/visualizer/api/session", nil)
		request.Host = tc.host
		request.Header.Set("Origin", tc.origin)
		request.Header.Set("Authorization", "Bearer "+testToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Errorf("host %s origin %s: got %d want %d", tc.host, tc.origin, response.Code, tc.want)
		}
		if response.Code == 403 && strings.Contains(response.Body.String(), tc.host) {
			t.Fatal("error echoed supplied Host")
		}
	}
}
