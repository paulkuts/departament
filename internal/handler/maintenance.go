package handler

import (
	"html"
	"net/http"
	"strings"

	"mitm-departament/internal/models"

	"github.com/gin-gonic/gin"
)

// emailAllowed сообщает, пускает ли режим «сайт в разработке» эту учётную запись.
func (h *Handler) emailAllowed(email *string) bool {
	if !h.maintenance.Enabled {
		return true
	}
	if email == nil {
		return false
	}
	return h.allowedEmails[strings.ToLower(strings.TrimSpace(*email))]
}

// clientIPAllowed — необязательный список адресов (например, IP рабочего ПК).
func (h *Handler) clientIPAllowed(c *gin.Context) bool {
	if len(h.maintenance.AllowedIPs) == 0 {
		return false
	}
	ip := c.ClientIP()
	for _, allowed := range h.maintenance.AllowedIPs {
		if strings.TrimSpace(allowed) == ip {
			return true
		}
	}
	return false
}

// maintenanceUser узнаёт пользователя по access-токену, а при загрузке страницы —
// по refresh-cookie (её браузер присылает сам, заголовка Authorization там нет).
func (h *Handler) maintenanceUser(c *gin.Context) *models.User {
	ctx := c.Request.Context()
	if token, err := getAccessToken(c); err == nil {
		if id, _, err := h.auth.svc.ParseToken(ctx, token); err == nil {
			if user, err := h.userSvc.GetByID(ctx, id); err == nil && user != nil && user.IsActive {
				return user
			}
		}
	}
	if tokenID, err := getRefreshToken(c); err == nil {
		if id, err := h.auth.svc.SessionUserID(ctx, tokenID); err == nil {
			if user, err := h.userSvc.GetByID(ctx, id); err == nil && user != nil && user.IsActive {
				return user
			}
		}
	}
	return nil
}

// maintenanceGate закрывает сайт от посторонних: наружу отдаётся страница
// «Сайт в разработке», API отвечает 503, доступ остаётся только у разрешённых
// учётных записей и (если задан) разрешённых IP.
func (h *Handler) maintenanceGate(c *gin.Context) {
	if !h.maintenance.Enabled {
		c.Next()
		return
	}

	path := c.Request.URL.Path
	// Вход, обновление сессии и статика нужны, чтобы администратор мог войти.
	if path == "/health" ||
		path == "/api/v1/auth/signin" ||
		path == "/api/v1/auth/register" ||
		path == "/api/v1/auth/refresh" ||
		path == "/api/v1/auth/logout" ||
		strings.HasPrefix(path, "/js/") ||
		strings.HasPrefix(path, "/css/") ||
		strings.HasPrefix(path, "/img/") ||
		strings.HasPrefix(path, "/favicon") {
		c.Next()
		return
	}

	if h.clientIPAllowed(c) {
		c.Next()
		return
	}
	if user := h.maintenanceUser(c); user != nil && h.emailAllowed(user.Email) {
		c.Next()
		return
	}

	if strings.HasPrefix(path, "/api/") {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, ErrorResponse{Error: "Сайт в разработке"})
		return
	}
	h.serveMaintenance(c)
}

// serveMaintenance отдаёт встроенную страницу-заглушку.
func (h *Handler) serveMaintenance(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.frontendFSReady {
		c.String(http.StatusServiceUnavailable, "Сайт в разработке")
		return
	}
	page, err := h.frontendFS.ReadFile("frontend/maintenance.html")
	if err != nil {
		c.String(http.StatusServiceUnavailable, "Сайт в разработке")
		return
	}
	title := h.maintenance.Title
	if title == "" {
		title = "Сайт в разработке"
	}
	body := strings.NewReplacer(
		"{{TITLE}}", html.EscapeString(title),
		"{{MESSAGE}}", html.EscapeString(h.maintenance.Message),
	).Replace(string(page))
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(body))
}

// registrationGate: открытая регистрация выключена — учётные записи создаёт администратор.
func (h *Handler) registrationGate(c *gin.Context) {
	if h.maintenance.RegistrationOpen {
		c.Next()
		return
	}
	c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{Error: "регистрация закрыта: учётную запись создаёт администратор"})
}
