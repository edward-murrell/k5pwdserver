package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gopkg.in/jcmturner/gokrb5.v7/config"
)

func TestIndexPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := Setup(&config.Config{})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content type = %q", ct)
	}
	if !strings.Contains(w.Body.String(), `id="app"`) {
		t.Error("page does not contain the app mount point")
	}
}
