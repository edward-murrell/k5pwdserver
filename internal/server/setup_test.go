package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gopkg.in/jcmturner/gokrb5.v7/config"
)

func get(t *testing.T, opts Options, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router, err := Setup(&config.Config{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func getIndex(t *testing.T, opts Options) *httptest.ResponseRecorder {
	t.Helper()
	return get(t, opts, "/")
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
	if !strings.Contains(body, `<footer class="footer">k5pwdserver dev</footer>`) {
		t.Error("footer does not show the package and version")
	}
	if !strings.Contains(body, `<script src="`+vuePath+`">`) {
		t.Error("page does not load the bundled Vue")
	}
	if strings.Contains(body, "unpkg.com") {
		t.Error("page still loads code from unpkg")
	}
}

func TestIndexPageTitle(t *testing.T) {
	body := getIndex(t, Options{Title: "Acme <Passwords>"}).Body.String()
	want := "Acme &lt;Passwords&gt;"
	if !strings.Contains(body, "<title>"+want+"</title>") || !strings.Contains(body, "<h2>"+want+"</h2>") {
		t.Error("custom title is missing or not escaped")
	}
}

func TestVueAsset(t *testing.T) {
	w := get(t, Options{}, vuePath)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("content type = %q", ct)
	}
	if !strings.HasPrefix(w.Body.String(), "/**\n* vue v3.5.44\n") {
		t.Error("asset is not Vue 3.5.44")
	}
}
