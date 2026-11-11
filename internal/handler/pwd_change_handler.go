package handler

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/edward-murrell/k5pwdserver/internal/kpasswd"
	"github.com/edward-murrell/k5pwdserver/internal/model"
	"github.com/edward-murrell/k5pwdserver/internal/ratelimit"
	"github.com/gin-gonic/gin"
	"gopkg.in/jcmturner/gokrb5.v7/client"
	"gopkg.in/jcmturner/gokrb5.v7/config"
)

type PasswordHandler struct {
	krbConfig     *config.Config
	allowedRealms []string
	limits        Limits
}

// Limits caps failed login attempts. Only attempts the KDC rejects for bad
// credentials count. A nil limiter doesn't limit.
type Limits struct {
	// User is keyed by the lower-cased username and the realm.
	User *ratelimit.Limiter
	// IP is keyed by the client's IP address.
	IP *ratelimit.Limiter
}

// NewPasswordHandler returns a handler for the given Kerberos config. If
// allowedRealms is not empty, only those realms are offered and accepted.
func NewPasswordHandler(cfg *config.Config, allowedRealms []string, limits Limits) *PasswordHandler {
	return &PasswordHandler{
		krbConfig:     cfg,
		allowedRealms: allowedRealms,
		limits:        limits,
	}
}

// realms returns the allowed realms, or every realm in krb5.conf if no limit is set.
func (h *PasswordHandler) realms() []string {
	if len(h.allowedRealms) > 0 {
		return h.allowedRealms
	}
	realms := make([]string, 0, len(h.krbConfig.Realms))
	for _, realm := range h.krbConfig.Realms {
		realms = append(realms, realm.Realm)
	}
	return realms
}

// defaultRealm returns krb5.conf's default realm. If that realm isn't allowed,
// it returns the first allowed realm.
func (h *PasswordHandler) defaultRealm() string {
	def := h.krbConfig.LibDefaults.DefaultRealm
	if len(h.allowedRealms) == 0 || h.isAllowed(def) {
		return def
	}
	return h.allowedRealms[0]
}

func (h *PasswordHandler) isAllowed(realm string) bool {
	return len(h.allowedRealms) == 0 || slices.Contains(h.allowedRealms, realm)
}

func (h *PasswordHandler) GetRealms(c *gin.Context) {
	realms := h.realms()

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
		DefaultRealm: h.defaultRealm(),
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
		realm = h.defaultRealm()
	}
	if !h.isAllowed(realm) {
		c.JSON(http.StatusBadRequest, model.Response{
			Success: false,
			Message: "Realm is not allowed",
		})
		return
	}

	// Reserve an attempt before contacting the KDC. It's handed back below
	// unless the KDC rejects the credentials.
	// Usernames are case-insensitive on AD, so they're lower-cased here.
	userKey := strings.ToLower(req.Username) + "@" + realm
	ipKey := c.ClientIP()
	if !h.takeAttempt(c, userKey, ipKey) {
		return
	}
	failedLogin := false
	defer func() {
		if !failedLogin {
			h.limits.User.Refund(userKey)
			h.limits.IP.Refund(ipKey)
		}
	}()

	// Create a new client instance with the user's current credentials
	cl := client.NewClientWithPassword(
		req.Username,
		realm,
		req.OldPassword,
		h.krbConfig,
		client.DisablePAFXFAST(true),
	)

	// Change the password. This also checks the current password, and needs no
	// TGT, so users whose password has expired can use it. gokrb5's
	// cl.ChangePasswd sends a target principal, which AD/Samba treat as an
	// admin reset and reject; see kpasswd.
	if err := kpasswd.ChangePassword(cl, req.NewPassword); err != nil {
		var lerr *kpasswd.LoginError
		if errors.As(err, &lerr) {
			failedLogin = true
			c.JSON(http.StatusUnauthorized, model.Response{
				Success: false,
				Message: "Invalid current credentials",
			})
			return
		}
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

// takeAttempt reserves an attempt for the user and the client IP. If either
// has no attempts left, it writes a 429 and returns false.
func (h *PasswordHandler) takeAttempt(c *gin.Context, userKey, ipKey string) bool {
	if ok, wait := h.limits.IP.Take(ipKey); !ok {
		log.Printf("Too many failed attempts from %s", ipKey)
		tooManyAttempts(c, wait)
		return false
	}
	if ok, wait := h.limits.User.Take(userKey); !ok {
		h.limits.IP.Refund(ipKey)
		log.Printf("Too many failed attempts for %s, latest from %s", userKey, ipKey)
		tooManyAttempts(c, wait)
		return false
	}
	return true
}

func tooManyAttempts(c *gin.Context, wait time.Duration) {
	c.Header("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
	minutes := int(math.Ceil(wait.Minutes()))
	unit := "minutes"
	if minutes <= 1 {
		minutes, unit = 1, "minute"
	}
	c.JSON(http.StatusTooManyRequests, model.Response{
		Success: false,
		Message: fmt.Sprintf("Too many failed attempts. Try again in %d %s.", minutes, unit),
	})
}
