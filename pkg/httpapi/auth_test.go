package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/splattner/vdcgo/pkg/httpapi"
	"github.com/splattner/vdcgo/pkg/runtime"
	"github.com/splattner/vdcgo/pkg/vdcapi"
)

func newAuthTestServer(t *testing.T, username, password string) string {
	t.Helper()
	state := vdcapi.NewStateStore()
	state.HandleEvent(runtime.Event{Type: runtime.EventInit, Output: "light", Name: "Test Light", UniqueID: "test-uid-001"})

	cfg := httpapi.Config{
		Listen:       "127.0.0.1:0",
		DSUID:        "AABBCCDDEEFF00112233445566778899AA",
		Description:  "test vdc",
		State:        state,
		Config:       vdcapi.NewConfigStore(),
		Scenes:       vdcapi.NewSceneStore(),
		AuthUsername: username,
		AuthPassword: password,
	}
	srv := httpapi.New(cfg)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts.URL
}

func TestAuthDisabledByDefault(t *testing.T) {
	url := newAuthTestServer(t, "", "")
	resp, err := http.Get(url + "/api/devices")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 when no password is configured", resp.StatusCode)
	}
}

func TestAuthRequiresCredentialsWhenEnabled(t *testing.T) {
	url := newAuthTestServer(t, "admin", "s3cret")

	resp, err := http.Get(url + "/api/devices")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for an unauthenticated request", resp.StatusCode)
	}
	if got := resp.Header.Get("WWW-Authenticate"); got == "" {
		t.Error("expected WWW-Authenticate header on 401 response")
	}
}

func TestAuthRejectsWrongCredentials(t *testing.T) {
	url := newAuthTestServer(t, "admin", "s3cret")

	req, err := http.NewRequest(http.MethodGet, url+"/api/devices", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("admin", "wrong-password")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for wrong password", resp.StatusCode)
	}
}

func TestAuthAcceptsCorrectCredentials(t *testing.T) {
	url := newAuthTestServer(t, "admin", "s3cret")

	req, err := http.NewRequest(http.MethodGet, url+"/api/devices", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("admin", "s3cret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 for correct credentials", resp.StatusCode)
	}
}

func TestAuthHealthEndpointAlwaysOpen(t *testing.T) {
	url := newAuthTestServer(t, "admin", "s3cret")

	resp, err := http.Get(url + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want /api/health to stay open even when auth is enabled", resp.StatusCode)
	}
}

func TestAuthGatesStaticUI(t *testing.T) {
	url := newAuthTestServer(t, "admin", "s3cret")

	resp, err := http.Get(url + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want the static UI to require auth too when enabled", resp.StatusCode)
	}
}
