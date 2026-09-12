package handler

import (
	"context"
	"mitm-departament/internal/config"
	"mitm-departament/internal/models"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AuthService interface {
	SignIn(ctx context.Context, email, password string) (models.TokenOutput, error)
	Logout(ctx context.Context, tokenID string) error
	ValidateRefreshSession(ctx context.Context, tokenID string) error
	ParseToken(ctx context.Context, accessToken string) (string, string, error)
	RefreshToken(ctx context.Context, tokenID string) (models.TokenOutput, error)
}

type AuthHandler struct {
	svc AuthService
	cfg config.AuthCfg
	log *zap.Logger
}

func NewAuthHandler(svc AuthService, cfg config.AuthCfg, log *zap.Logger) *AuthHandler {
	return &AuthHandler{svc: svc, cfg: cfg, log: log}
}

func (h *AuthHandler) RegisterRoutes(rg *gin.RouterGroup) {
	auth := rg.Group("/auth")
	{
		auth.POST("/signin", h.signIn) // ← фикс
		auth.POST("/refresh", h.refresh)
		auth.POST("/logout", h.logout)
	}
}

func (h *AuthHandler) signIn(c *gin.Context) {
	var signIn UserSignIn
	if err := c.ShouldBindJSON(&signIn); err != nil {
		h.log.Debug("sign in failed: invalid password",
			zap.String("client_ip", c.ClientIP()),
			zap.Error(err),
		)
		handleValidationError(c, err)
		return
	}

	user, err := h.svc.SignIn(c.Request.Context(), strings.ToLower(strings.TrimSpace(signIn.Email)), signIn.Password)
	if err != nil {
		h.log.Debug("sign in failed: invalid credentials",
			zap.String("client_ip", c.ClientIP()),
			zap.Error(err),
		)
		handleError(c, err)
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshToken,
		Value:    user.RefreshToken,
		MaxAge:   int(h.cfg.RefreshTokenTTL.Seconds()),
		Path:     "/",
		Secure:   true, // production разворачивается только за HTTPS
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	c.JSON(http.StatusOK, gin.H{accessToken: user.AccessToken})
}

func (h *AuthHandler) logout(c *gin.Context) {
	refreshTkn, err := getRefreshToken(c)
	if err != nil {
		h.log.Debug("logout failed: no refresh token",
			zap.String("client_ip", c.ClientIP()),
			zap.Error(err),
		)
		handleError(c, err)
		return
	}

	if err := h.svc.Logout(c.Request.Context(), refreshTkn); err != nil {
		h.log.Error("logout failed due to internal error",
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		handleError(c, err)
		return
	}

	h.log.Info("user logged out successfully",
		zap.String("client_ip", c.ClientIP()),
	)

	c.JSON(http.StatusOK, MessageResponse{Message: "logout successful"})
}

func (h *AuthHandler) refresh(c *gin.Context) {
	refreshTkn, err := getRefreshToken(c)
	if err != nil {
		h.log.Debug("refresh failed: no refresh token",
			zap.String("client_ip", c.ClientIP()),
			zap.Error(err),
		)
		handleError(c, err)
		return
	}

	token, err := h.svc.RefreshToken(c.Request.Context(), refreshTkn)
	if err != nil {
		h.log.Error("refresh token failed due to internal error",
			zap.Error(err),
			zap.String("client_ip", c.ClientIP()),
		)
		handleError(c, err)
		return
	}

	h.log.Info("token refreshed successfully",
		zap.String("client_ip", c.ClientIP()),
	)

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshToken,
		Value:    token.RefreshToken,
		MaxAge:   int(h.cfg.RefreshTokenTTL.Seconds()),
		Path:     "/",
		Secure:   true, // production разворачивается только за HTTPS
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	c.JSON(http.StatusOK, gin.H{accessToken: token.AccessToken})
}
