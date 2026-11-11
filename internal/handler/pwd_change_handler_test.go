package handler

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/edward-murrell/k5pwdserver/internal/kdctest"
	"github.com/edward-murrell/k5pwdserver/internal/model"
	"github.com/edward-murrell/k5pwdserver/internal/ratelimit"
	"github.com/gin-gonic/gin"
	"gopkg.in/jcmturner/gokrb5.v7/config"
	"gopkg.in/jcmturner/gokrb5.v7/iana/errorcode"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// testConfig returns a krb5 config whose KDCs point at a closed local port.
func testConfig(t *testing.T, realms string) *config.Config {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	kdc := ln.Addr().String()
	ln.Close()

	s := "[libdefaults]\n default_realm = EXAMPLE.COM\n dns_lookup_kdc = false\n udp_preference_limit = 1\n[realms]\n"
	for _, r := range strings.Fields(realms) {
		s += " " + r + " = {\n  kdc = " + kdc + "\n }\n"
	}
	cfg, err := config.NewConfigFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func serve(h *PasswordHandler, req *http.Request) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET("/api/realms", h.GetRealms)
	r.POST("/api/change-password", h.ChangePassword)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestGetRealms(t *testing.T) {
	h := NewPasswordHandler(testConfig(t, "EXAMPLE.COM OTHER.ORG"), nil, Limits{})
	w := serve(h, httptest.NewRequest(http.MethodGet, "/api/realms", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp model.RealmsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if strings.Join(resp.Realms, " ") != "EXAMPLE.COM OTHER.ORG" {
		t.Errorf("realms = %v", resp.Realms)
	}
	if resp.DefaultRealm != "EXAMPLE.COM" {
		t.Errorf("default realm = %q", resp.DefaultRealm)
	}
}

func TestGetRealmsNoneConfigured(t *testing.T) {
	h := NewPasswordHandler(testConfig(t, ""), nil, Limits{})
	w := serve(h, httptest.NewRequest(http.MethodGet, "/api/realms", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestChangePasswordStatus(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        int
	}{
		{"wrong content type", "text/plain", `{}`, http.StatusUnsupportedMediaType},
		{"form post", "application/x-www-form-urlencoded", "username=a", http.StatusUnsupportedMediaType},
		{"invalid json", "application/json", `{`, http.StatusBadRequest},
		{"unreachable KDC", "application/json",
			`{"username":"alice","oldPassword":"old","newPassword":"new"}`, http.StatusInternalServerError},
	}
	h := NewPasswordHandler(testConfig(t, "EXAMPLE.COM"), nil, Limits{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/change-password", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			w := serve(h, req)
			if w.Code != tt.want {
				t.Errorf("status = %d, want %d (body %s)", w.Code, tt.want, w.Body)
			}
		})
	}
}

func TestGetRealmsAllowed(t *testing.T) {
	tests := []struct {
		name        string
		allowed     []string
		wantRealms  string
		wantDefault string
	}{
		{"default allowed", []string{"OTHER.ORG", "EXAMPLE.COM"}, "OTHER.ORG EXAMPLE.COM", "EXAMPLE.COM"},
		{"default not allowed", []string{"OTHER.ORG"}, "OTHER.ORG", "OTHER.ORG"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewPasswordHandler(testConfig(t, "EXAMPLE.COM OTHER.ORG THIRD.NET"), tt.allowed, Limits{})
			w := serve(h, httptest.NewRequest(http.MethodGet, "/api/realms", nil))
			var resp model.RealmsResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(resp.Realms, " "); got != tt.wantRealms {
				t.Errorf("realms = %q, want %q", got, tt.wantRealms)
			}
			if resp.DefaultRealm != tt.wantDefault {
				t.Errorf("default realm = %q, want %q", resp.DefaultRealm, tt.wantDefault)
			}
		})
	}
}

func TestChangePasswordRealmNotAllowed(t *testing.T) {
	h := NewPasswordHandler(testConfig(t, "EXAMPLE.COM OTHER.ORG"), []string{"OTHER.ORG"}, Limits{})
	for _, body := range []string{
		`{"username":"alice","realm":"EXAMPLE.COM","oldPassword":"old","newPassword":"new"}`,
		`{"username":"alice","realm":"example.com","oldPassword":"old","newPassword":"new"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/change-password", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := serve(h, req)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "not allowed") {
			t.Errorf("body %s: status = %d (%s), want 400 not allowed", body, w.Code, w.Body)
		}
	}
}

func TestChangePasswordRejectedCredentials(t *testing.T) {
	kdc := kdctest.Start(t, "EXAMPLE.COM", errorcode.KDC_ERR_PREAUTH_FAILED)
	h := NewPasswordHandler(kdc.Config(t), nil, Limits{})

	req := httptest.NewRequest(http.MethodPost, "/api/change-password",
		strings.NewReader(`{"username":"alice","oldPassword":"wrong","newPassword":"new"}`))
	req.Header.Set("Content-Type", "application/json")
	w := serve(h, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (body %s)", w.Code, w.Body)
	}
	// Asking for a TGT would fail for users whose password has expired.
	for _, svc := range kdc.Services() {
		if svc != "kadmin/changepw" {
			t.Errorf("handler requested a ticket for %s, want only kadmin/changepw", svc)
		}
	}
}

func changeRequest(user, remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/change-password",
		strings.NewReader(`{"username":"`+user+`","oldPassword":"wrong","newPassword":"new"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	return req
}

func TestUserLimit(t *testing.T) {
	kdc := kdctest.Start(t, "EXAMPLE.COM", errorcode.KDC_ERR_PREAUTH_FAILED)
	h := NewPasswordHandler(kdc.Config(t), nil, Limits{User: ratelimit.New(2, time.Minute)})

	for i, want := range []int{http.StatusUnauthorized, http.StatusUnauthorized, http.StatusTooManyRequests} {
		w := serve(h, changeRequest("alice", "192.0.2.1:1000"))
		if w.Code != want {
			t.Fatalf("attempt %d: status = %d, want %d", i+1, w.Code, want)
		}
	}
	asked := len(kdc.Services())

	// Usernames are matched case-insensitively, and from any IP.
	w := serve(h, changeRequest("ALICE", "192.0.2.99:1000"))
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("ALICE from another IP: status = %d, want 429", w.Code)
	}
	if ra := w.Header().Get("Retry-After"); ra == "" || ra == "0" {
		t.Errorf("Retry-After = %q, want seconds until the window ends", ra)
	}
	if len(kdc.Services()) != asked {
		t.Error("a limited attempt still reached the KDC")
	}

	if w := serve(h, changeRequest("bob", "192.0.2.1:1000")); w.Code != http.StatusUnauthorized {
		t.Errorf("other user: status = %d, want 401", w.Code)
	}
}

func TestIPLimit(t *testing.T) {
	kdc := kdctest.Start(t, "EXAMPLE.COM", errorcode.KDC_ERR_PREAUTH_FAILED)
	h := NewPasswordHandler(kdc.Config(t), nil, Limits{IP: ratelimit.New(2, time.Minute)})

	for i, user := range []string{"alice", "bob"} {
		if w := serve(h, changeRequest(user, "192.0.2.1:1000")); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, w.Code)
		}
	}
	if w := serve(h, changeRequest("carol", "192.0.2.1:2000")); w.Code != http.StatusTooManyRequests {
		t.Errorf("third user from the same IP: status = %d, want 429", w.Code)
	}
	if w := serve(h, changeRequest("carol", "192.0.2.2:1000")); w.Code != http.StatusUnauthorized {
		t.Errorf("other IP: status = %d, want 401", w.Code)
	}
}

func TestLimitIgnoresOtherFailures(t *testing.T) {
	// A KDC fault isn't a failed login, so it mustn't use up attempts.
	kdc := kdctest.Start(t, "EXAMPLE.COM", errorcode.KDC_ERR_ETYPE_NOSUPP)
	h := NewPasswordHandler(kdc.Config(t), nil, Limits{
		User: ratelimit.New(1, time.Minute),
		IP:   ratelimit.New(1, time.Minute),
	})
	for i := 0; i < 3; i++ {
		if w := serve(h, changeRequest("alice", "192.0.2.1:1000")); w.Code != http.StatusInternalServerError {
			t.Fatalf("attempt %d: status = %d, want 500", i+1, w.Code)
		}
	}
}
