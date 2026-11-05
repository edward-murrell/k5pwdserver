package server

import (
	_ "embed"
	"net/http"

	"github.com/edward-murrell/k8init/internal/handler"
	"github.com/gin-gonic/gin"
	"gopkg.in/jcmturner/gokrb5.v7/config"
)

//go:embed web/index.html
var indexHTML []byte

// Options holds the server settings that come from the command line.
type Options struct {
	// Realms limits which realms users may pick. Empty allows every realm.
	Realms []string
}

func Setup(krbConfig *config.Config, opts Options) *gin.Engine {
	router := gin.Default()

	// The UI is served from the same origin as the API, so no CORS headers are
	// sent and browsers will block cross-site scripts from calling the API.
	router.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})

	passwordHandler := handler.NewPasswordHandler(krbConfig, opts.Realms)
	router.GET("/api/realms", passwordHandler.GetRealms)
	router.POST("/api/change-password", passwordHandler.ChangePassword)

	return router
}
