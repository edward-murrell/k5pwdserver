package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gopkg.in/jcmturner/gokrb5.v7/config"
)

func getIndex(t *testing.T, opts Options) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router, err := Setup(&config.Config{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	return w
}

func TestIndexPage(t *testing.T) {
	w := getIndex(t, Options{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content type = %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `id="app"`) {
		t.Error("page does not contain the app mount point")
	}
	if !strings.Contains(body, "{{ realm }}") {
		t.Error("Vue template syntax was not passed through")
	}
	if !strings.Contains(body, "<title>"+DefaultTitle+"</title>") {
		t.Error("page does not use the default title")
	}
	if !strings.Contains(body, `<footer class="footer">k8init dev</footer>`) {
		t.Error("footer does not show the package and version")
	}
}

func TestIndexPageTitle(t *testing.T) {
	body := getIndex(t, Options{Title: "Acme <Passwords>"}).Body.String()
	want := "Acme &lt;Passwords&gt;"
	if !strings.Contains(body, "<title>"+want+"</title>") || !strings.Contains(body, "<h2>"+want+"</h2>") {
		t.Error("custom title is missing or not escaped")
	}
}
