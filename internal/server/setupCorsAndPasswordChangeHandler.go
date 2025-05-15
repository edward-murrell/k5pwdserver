package server

import (
	"github.com/edward-murrell/k8init/internal/handler"
	"github.com/gin-gonic/gin"
	"gopkg.in/jcmturner/gokrb5.v7/config"
)

func Setup(krbConfig *config.Config) *gin.Engine {
	router := gin.Default()

	// Enable CORS
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(200)
			return
		}

		c.Next()
	})

	passwordHandler := handler.NewPasswordHandler(krbConfig)
	router.POST("/api/change-password", passwordHandler.ChangePassword)

	return router
}
