package handler

import (
	"context"
	"errors"
	"fmt"
	contextkeys "mitm-departament/internal/contextKey"
	"mitm-departament/pkg/ratelimiter"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	requestIDKey = "request_id"
	userIDKey    = "user_id"
	roleKey      = "role"
	adminKey     = "admin"
	userKey      = "user"
	refreshToken = "refresh_token"
	accessToken  = "access_token"
	authHeader   = "Authorization"
)

// rateLimitMiddleware создает middleware для ограничения частоты запросов.
func (h *Handler) rateLimitMiddleware(limiter *ratelimiter.RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()

		if !limiter.Allow(clientIP) {
			h.log.Warn("rate limit exceeded",
				zap.String("client_ip", clientIP),
				zap.String("path", c.Request.URL.Path),
			)
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "too_many_requests",
				"message": "Превышен лимит запросов. Пожалуйста, подождите.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (s *Handler) logging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestID := uuid.New().String()

		c.Set(requestIDKey, requestID)
		ctx := context.WithValue(c.Request.Context(), contextkeys.RequestIDKey, requestID)
		c.Request = c.Request.WithContext(ctx)

		path := c.Request.URL.Path
		method := c.Request.Method
		clientIP := c.ClientIP()

		s.log.Debug("HTTP request started",
			zap.String("request_id", requestID),
			zap.String("method", method),
			zap.String("path", path),
			zap.String("client_ip", clientIP),
		)

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		bodySize := c.Writer.Size()
		if bodySize < 0 {
			bodySize = 0
		}

		fields := []zap.Field{
			zap.String("request_id", requestID),
			zap.Int("status_code", statusCode),
			zap.String("method", method),
			zap.String("path", path),
			zap.String("client_ip", clientIP),
			zap.Duration("latency_ms", latency),
			zap.Int("response_size_bytes", bodySize),
		}

		var logFn func(string, ...zap.Field)
		switch {
		case statusCode >= 500:
			logFn = s.log.Error
		case statusCode >= 400:
			logFn = s.log.Warn
		default:
			logFn = s.log.Info
		}

		logFn("http_request_completed", fields...)
	}
}

func (s *Handler) loggerWith(c *gin.Context, fields ...zap.Field) *zap.Logger {
	base := []zap.Field{
		zap.String("request_id", s.getRequestID(c)),
		zap.String("client_ip", c.ClientIP()),
	}
	return s.log.With(append(base, fields...)...)
}

func (s *Handler) getRequestID(c *gin.Context) string {
	return c.GetString(requestIDKey)
}

func (h *Handler) authMiddleware(c *gin.Context) {
	// 1. Достаём access token из заголовка
	token, err := getAccessToken(c)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized,
			ErrorResponse{Error: "отсутствует токен доступа"})
		return
	}

	// 2. Валидируем JWT (без обращения к БД)
	userID, _, err := h.auth.svc.ParseToken(c.Request.Context(), token)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized,
			ErrorResponse{Error: "недействительный токен"})
		return
	}

	user, err := h.userSvc.GetByID(c.Request.Context(), userID)
	if err != nil || user == nil || !user.IsActive {
		c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "учётная запись недоступна"})
		return
	}
	// Current database permissions override JWT claims.
	c.Set(userIDKey, userID)
	c.Set(roleKey, user.Role)
	c.Next()
}

// optionalAuth узнаёт сотрудника, если он передал корректный токен доступа, но
// не мешает гостю: без токена запрос продолжается как гостевой.
func (h *Handler) optionalAuth(c *gin.Context) {
	token, err := getAccessToken(c)
	if err != nil {
		c.Next()
		return
	}
	userID, _, err := h.auth.svc.ParseToken(c.Request.Context(), token)
	if err != nil {
		c.Next()
		return
	}
	user, err := h.userSvc.GetByID(c.Request.Context(), userID)
	if err != nil || user == nil || !user.IsActive {
		c.Next()
		return
	}
	c.Set(userIDKey, userID)
	c.Set(roleKey, user.Role)
	c.Next()
}

func getAccessToken(c *gin.Context) (string, error) {
	token := c.GetHeader(authHeader)
	if token == "" {
		return "", errors.New("empty authorization header")
	}

	tokenPaths := strings.Split(token, " ")
	if len(tokenPaths) != 2 || tokenPaths[0] != "Bearer" {
		return "", errors.New("invalid authorization header format")
	}

	return tokenPaths[1], nil
}

func getUserID(c *gin.Context) (uuid.UUID, error) {
	id, exists := c.Get(userIDKey)
	if !exists {
		return uuid.Nil, fmt.Errorf("user id not found in context")
	}

	t, ok := id.(string)
	if !ok {
		return uuid.Nil, fmt.Errorf("invalid user id format")
	}

	userID, err := uuid.Parse(t)
	if err != nil {
		return uuid.Nil, fmt.Errorf("user id is not uuid")
	}

	if userID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("invalid user id")
	}

	return userID, nil
}

func getRefreshToken(c *gin.Context) (string, error) {
	tokenID, err := c.Cookie(refreshToken)
	if err != nil || tokenID == "" {
		return "", fmt.Errorf("token ID not found in cookie: %v", err)
	}

	return tokenID, nil
}

func getParamUUID(c *gin.Context, param string) (string, error) {
	id := c.Param(param)
	if id == "" {
		return "", fmt.Errorf("empty %s param", param)
	}
	return id, nil
}

func requireRoles(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		raw, exists := c.Get(roleKey)
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden,
				ErrorResponse{Error: "доступ запрещён"})
			return
		}
		role, ok := raw.(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden,
				ErrorResponse{Error: "доступ запрещён"})
			return
		}
		if _, ok := allowed[role]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden,
				ErrorResponse{Error: "недостаточно прав"})
			return
		}
		c.Next()
	}
}
