package server

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"net/http"

	"github.com/edward-murrell/k5pwdserver/internal/handler"
	"github.com/edward-murrell/k5pwdserver/internal/version"
	"github.com/gin-gonic/gin"
	"gopkg.in/jcmturner/gokrb5.v7/config"
)

// DefaultTitle is the page title when none is configured.
const DefaultTitle = "Change Kerberos Password"

//go:embed web/index.html
var indexSource string

// Vue is bundled so the page works without internet access and doesn't load
// code from a third-party CDN. To upgrade, replace the file and update both names.
//
//go:embed web/vue-3.5.44.global.prod.js
var vueJS []byte

const vuePath = "/assets/vue-3.5.44.global.prod.js"

// The page uses Vue's {{ }} syntax, so the Go template uses [[ ]] instead.
var indexTemplate = template.Must(template.New("index").Delims("[[", "]]").Parse(indexSource))

// Options holds the server settings that come from the command line.
type Options struct {
	// Realms limits which realms users may pick. Empty allows every realm.
	Realms []string
	// Title is shown in the browser tab and as the page heading.
	Title string
	// Limits caps failed login attempts per user and per client IP.
	Limits handler.Limits
	// TrustedProxies lists proxy IPs or CIDRs whose X-Forwarded-For header
	// gives the client IP. Empty trusts no proxies.
	TrustedProxies []string
}

func Setup(krbConfig *config.Config, opts Options) (*gin.Engine, error) {
	if opts.Title == "" {
		opts.Title = DefaultTitle
	}
	data := struct {
		Title, Package, Version, VuePath string
	}{opts.Title, version.Package, version.Version, vuePath}
	var page bytes.Buffer
	if err := indexTemplate.Execute(&page, data); err != nil {
		return nil, err
	}
	indexHTML := page.Bytes()

	router := gin.Default()
	// Gin trusts X-Forwarded-For from anyone by default, which would let
	// clients choose their own IP and dodge the per-IP limit.
	if err := router.SetTrustedProxies(opts.TrustedProxies); err != nil {
		return nil, fmt.Errorf("trusted proxies: %w", err)
	}

	// The UI is served from the same origin as the API, so no CORS headers are
	// sent and browsers will block cross-site scripts from calling the API.
	router.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})
	router.GET(vuePath, func(c *gin.Context) {
		// The version is in the path, so browsers can cache it for good.
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Data(http.StatusOK, "text/javascript; charset=utf-8", vueJS)
	})

	passwordHandler := handler.NewPasswordHandler(krbConfig, opts.Realms, opts.Limits)
	router.GET("/api/realms", passwordHandler.GetRealms)
	router.POST("/api/change-password", passwordHandler.ChangePassword)

	return router, nil
}
