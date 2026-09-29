package gomcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestConnectionReportsOnlyAuthenticatedIdentity(t *testing.T) {
	server, store := testServer(t, true)
	deviceToken := strings.Repeat("d", 40)
	entry, err := store.AddClientToken(context.Background(), "registered-laptop", sha256.Sum256([]byte(deviceToken)))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		token, role, device string
		status              int
	}{
		{testToken, "owner", "", 200}, {deviceToken, "device", "registered-laptop", 200}, {"", "", "", 401}, {strings.Repeat("x", 40), "", "", 401},
	} {
		req, _ := http.NewRequest("GET", server.URL+"/connection", nil)
		if item.token != "" {
			req.Header.Set("Authorization", "Bearer "+item.token)
		}
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Role   string `json:"role"`
			Device string `json:"device"`
		}
		if response.StatusCode == 200 {
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
		}
		response.Body.Close()
		if response.StatusCode != item.status || result.Role != item.role || result.Device != item.device {
			t.Fatalf("identity: status=%d role=%q device=%q", response.StatusCode, result.Role, result.Device)
		}
	}
	if _, err := store.RevokeClientToken(context.Background(), entry.ID); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", server.URL+"/connection", nil)
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("revoked device remained authenticated")
	}
}
