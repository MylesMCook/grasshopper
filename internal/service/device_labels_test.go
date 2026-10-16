package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestOwnerRenamesADeviceAndOnlyTheOwnerCan(t *testing.T) {
	server, store := testServer(t, true)
	paired := "synthetic-paired-device-token-0123456789"
	if _, err := store.AddClientToken(context.Background(), "mc-pc", sha256.Sum256([]byte(paired))); err != nil {
		t.Fatal(err)
	}
	call := func(method, token, origin, body string) (int, []byte) {
		t.Helper()
		req, _ := http.NewRequest(method, server.URL+"/visualizer/api/devices", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var data []byte
		data, _ = json.Marshal(nil)
		if resp.StatusCode == 200 {
			var decoded any
			_ = json.NewDecoder(resp.Body).Decode(&decoded)
			data, _ = json.Marshal(decoded)
		}
		return resp.StatusCode, data
	}
	rename := `{"device":"mc-pc","label":"Gaming PC"}`
	if status, _ := call(http.MethodPatch, paired, server.URL, rename); status != 401 {
		t.Fatalf("a paired device renamed itself: %d", status)
	}
	if status, _ := call(http.MethodPatch, testToken, "https://foreign.example", rename); status != 403 {
		t.Fatalf("a foreign Origin renamed a device: %d", status)
	}
	if status, _ := call(http.MethodPatch, testToken, server.URL, `{"device":"nobody","label":"x"}`); status != 404 {
		t.Fatalf("renaming an unknown device: %d", status)
	}
	if status, _ := call(http.MethodPatch, testToken, server.URL, `{"device":"mc-pc","label":"`+strings.Repeat("x", 65)+`"}`); status != 400 {
		t.Fatalf("an oversized name: %d", status)
	}
	if status, _ := call(http.MethodPatch, testToken, server.URL, rename); status != 204 {
		t.Fatalf("owner rename: %d", status)
	}
	status, body := call(http.MethodGet, testToken, "", "")
	if status != 200 || !strings.Contains(string(body), `"label":"Gaming PC"`) || !strings.Contains(string(body), `"device":"mc-pc"`) {
		t.Fatalf("devices after rename: %d %s", status, body)
	}
}
