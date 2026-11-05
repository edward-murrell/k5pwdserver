package handler

import (
	"errors"
	"net/http"

	"github.com/edward-murrell/k8init/internal/kpasswd"
	"github.com/edward-murrell/k8init/internal/model"
	"github.com/gin-gonic/gin"
	"gopkg.in/jcmturner/gokrb5.v7/client"
	"gopkg.in/jcmturner/gokrb5.v7/config"
)

type PasswordHandler struct {
	krbConfig *config.Config
}

func NewPasswordHandler(cfg *config.Config) *PasswordHandler {
	return &PasswordHandler{
		krbConfig: cfg,
	}
}

// Add this new method to the existing PasswordHandler struct

func (h *PasswordHandler) GetRealms(c *gin.Context) {
	realms := make([]string, 0)

	// Get all configured realms
	for _, realm := range h.krbConfig.Realms {
		realms = append(realms, realm.Realm)
	}

	// If no realms are configured, return an error
	if len(realms) == 0 {
		c.JSON(http.StatusInternalServerError, model.Response{
			Success: false,
			Message: "No Kerberos realms configured",
		})
		return
	}

	c.JSON(http.StatusOK, model.RealmsResponse{
		Realms:       realms,
		DefaultRealm: h.krbConfig.LibDefaults.DefaultRealm,
	})
}

func (h *PasswordHandler) ChangePassword(c *gin.Context) {
	// Requiring a JSON content type forces cross-origin browser requests through a
	// CORS preflight, which this server never approves.
	if c.ContentType() != "application/json" {
		c.JSON(http.StatusUnsupportedMediaType, model.Response{
			Success: false,
			Message: "Content-Type must be application/json",
		})
		return
	}

	var req model.PasswordChangeRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.Response{
			Success: false,
			Message: "Invalid request format",
		})
		return
	}

	// Use provided realm or fall back to default realm
	realm := req.Realm
	if realm == "" {
		realm = h.krbConfig.LibDefaults.DefaultRealm
	}

	// Create a new client instance with the user's current credentials
	cl := client.NewClientWithPassword(
		req.Username,
		realm,
		req.OldPassword,
		h.krbConfig,
		client.DisablePAFXFAST(true),
	)

	// Try to login first to verify current credentials
	err := cl.AffirmLogin()
	if err != nil {
		c.JSON(http.StatusUnauthorized, model.Response{
			Success: false,
			Message: "Invalid current credentials",
		})
		return
	}

	// Attempt to change the password. gokrb5's cl.ChangePasswd sends a target
	// principal, which AD/Samba treat as an admin reset and reject; see kpasswd.
	if err := kpasswd.ChangePassword(cl, req.NewPassword); err != nil {
		var kerr *kpasswd.ResultError
		if errors.As(err, &kerr) && kerr.IsPolicyRejection() {
			c.JSON(http.StatusBadRequest, model.Response{
				Success: false,
				Message: kerr.Message,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, model.Response{
			Success: false,
			Message: "Failed to change password: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, model.Response{
		Success: true,
		Message: "Password changed successfully",
	})

}
