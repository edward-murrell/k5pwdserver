package server

import (
	"bytes"
	_ "embed"
	"html/template"
	"net/http"

	"github.com/edward-murrell/k8init/internal/handler"
	"github.com/gin-gonic/gin"
	"gopkg.in/jcmturner/gokrb5.v7/config"
)

// DefaultTitle is the page title when none is configured.
const DefaultTitle = "Change Kerberos Password"

//go:embed web/index.html
var indexSource string

// The page uses Vue's {{ }} syntax, so the Go template uses [[ ]] instead.
var indexTemplate = template.Must(template.New("index").Delims("[[", "]]").Parse(indexSource))

// Options holds the server settings that come from the command line.
type Options struct {
	// Realms limits which realms users may pick. Empty allows every realm.
	Realms []string
	// Title is shown in the browser tab and as the page heading.
	Title string
}

func Setup(krbConfig *config.Config, opts Options) (*gin.Engine, error) {
	if opts.Title == "" {
		opts.Title = DefaultTitle
	}
	var page bytes.Buffer
	if err := indexTemplate.Execute(&page, opts); err != nil {
		return nil, err
	}
	indexHTML := page.Bytes()

	router := gin.Default()

	// The UI is served from the same origin as the API, so no CORS headers are
	// sent and browsers will block cross-site scripts from calling the API.
	router.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})

	passwordHandler := handler.NewPasswordHandler(krbConfig, opts.Realms)
	router.GET("/api/realms", passwordHandler.GetRealms)
	router.POST("/api/change-password", passwordHandler.ChangePassword)

	return router, nil
}
