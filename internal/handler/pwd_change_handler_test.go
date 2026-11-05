package handler

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edward-murrell/k8init/internal/model"
	"github.com/gin-gonic/gin"
	"gopkg.in/jcmturner/gokrb5.v7/config"
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
	h := NewPasswordHandler(testConfig(t, "EXAMPLE.COM OTHER.ORG"))
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
	h := NewPasswordHandler(testConfig(t, ""))
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
			`{"username":"alice","oldPassword":"old","newPassword":"new"}`, http.StatusUnauthorized},
	}
	h := NewPasswordHandler(testConfig(t, "EXAMPLE.COM"))
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
