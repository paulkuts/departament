package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// handleError определяет HTTP-код по тексту ошибки
func handleError(c *gin.Context, err error) {
	msg := err.Error()

	switch {
	case strings.Contains(msg, "invalid credentials"), strings.Contains(msg, "invalid session"), strings.Contains(msg, "session expired"):
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "неверные данные входа или сессия"})
	case strings.Contains(msg, "not holder"):
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "вернуть ключ может его держатель или администратор"})
	case strings.Contains(msg, "cannot delete"):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "нельзя удалить выданный ключ или ключ с историей"})
	case strings.Contains(msg, "not found"):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: msg})
	case strings.Contains(msg, "not available"),
		strings.Contains(msg, "not issued"),
		strings.Contains(msg, "not marked as lost"),
		strings.Contains(msg, "already marked"):
		c.JSON(http.StatusConflict, ErrorResponse{Error: msg})
	case strings.Contains(msg, "UNIQUE constraint failed"):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "запись с такими данными уже существует"})
	case strings.Contains(msg, "FOREIGN KEY constraint failed"):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ссылка на несуществующую запись"})
	default:
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "внутренняя ошибка сервера"})
	}
}

// handleValidationError обрабатывает ошибки валидации Gin
func handleValidationError(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
}

// parseIDParam парсит int64 из URL-параметра
func parseIDParam(c *gin.Context, param string) (int64, bool) {
	idStr := c.Param(param)
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "некорректный id"})
		return 0, false
	}
	return id, true
}
