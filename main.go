package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"gopkg.in/jcmturner/gokrb5.v7/client"
	"gopkg.in/jcmturner/gokrb5.v7/config"
)

type PasswordChangeRequest struct {
	Username    string `json:"username"`
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func main() {
	router := gin.Default()

	// Enable CORS
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	})

	// Load Kerberos configuration
	cfg, err := config.Load("/etc/krb5.conf")
	if err != nil {
		log.Fatalf("Error loading Kerberos config: %v", err)
	}

	router.POST("/api/change-password", func(c *gin.Context) {
		var req PasswordChangeRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, Response{
				Success: false,
				Message: "Invalid request format",
			})
			return
		}

		// Create a new client instance with the user's current credentials
		cl := client.NewClientWithPassword(
			req.Username,
			cfg.LibDefaults.DefaultRealm,
			req.OldPassword,
			cfg,
		)

		// Try to login first to verify current credentials
		err := cl.Login()
		if err != nil {
			c.JSON(http.StatusUnauthorized, Response{
				Success: false,
				Message: "Invalid current credentials",
			})
			return
		}

		// Attempt to change the password
		success, err := cl.ChangePasswd(req.NewPassword)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Success: false,
				Message: "Failed to change password: " + err.Error(),
			})
			return
		}

		if !success {
			c.JSON(http.StatusInternalServerError, Response{
				Success: false,
				Message: "Password change operation failed",
			})
			return
		}

		c.JSON(http.StatusOK, Response{
			Success: true,
			Message: "Password changed successfully",
		})
	})

	log.Printf("Server starting on :8080")
	log.Fatal(router.Run(":8080"))
}
