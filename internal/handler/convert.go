package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mitm-departament/internal/config"

	"github.com/gin-gonic/gin"
)

// Конвертер файлов: тонкий прокси к Transmute на домашнем сервере (CT102 на PVE,
// доступен по Tailscale). Ключ сервиса хранится только на сервере сайта, поэтому
// страница ходит через этот маршрут, а не в Transmute напрямую. Тяжёлая работа
// (LibreOffice, pandoc, ffmpeg) идёт на домашнем сервере, rf-vps не нагружается.
type ConvertHandler struct {
	cfg    config.ConvertConfig
	client *http.Client
}

func NewConvertHandler(cfg config.ConvertConfig) *ConvertHandler {
	return &ConvertHandler{cfg: cfg, client: &http.Client{Timeout: convertTimeout}}
}

// Разрешённые разделы Transmute: файлы, конвертации и проверка живости.
var convertAllowedPrefixes = []string{"/files", "/conversions", "/health"}

// Все маршруты движка живут под /api (см. его OpenAPI): /api/files, /api/conversions.
const convertUpstreamPrefix = "/api"

func (h *ConvertHandler) RegisterRoutes(rg *gin.RouterGroup) {
	// Роль не проверяем: любой вошедший сотрудник может конвертировать (решение
	// Павла 05.10.2026). Ключ движка всё равно остаётся только на сервере.
	rg.Any("/convert/*path", h.handle)
}

func (h *ConvertHandler) handle(c *gin.Context) {
	path := c.Param("path") // начинается с «/»
	// Статус отдаём сами, без обращения к домашнему серверу: страница узнаёт,
	// настроен ли раздел и какой у него предел размера файла.
	if path == "/status" {
		c.JSON(http.StatusOK, gin.H{
			"enabled":       h.cfg.BaseURL != "",
			"max_file_size": h.cfg.MaxFileSize,
		})
		return
	}

	if h.cfg.BaseURL == "" {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "конвертер не настроен"})
		return
	}
	if !convertPathAllowed(path) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "недоступный раздел конвертера"})
		return
	}
	if h.cfg.MaxFileSize > 0 && c.Request.ContentLength > int64(h.cfg.MaxFileSize) {
		c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{Error: fmt.Sprintf("файл слишком большой (макс. %d МБ)", h.cfg.MaxFileSize/(1024*1024))})
		return
	}

	// Файл может быть крупным и уходить по медленному каналу, поэтому на этом
	// маршруте снимаем общие дедлайны сервера (read_timeout 5s).
	extendDeadlines(c, convertTimeout)

	// Страховка на случай запроса без Content-Length (chunked): читаем не больше
	// предела плюс запас на границы multipart.
	body := c.Request.Body
	if h.cfg.MaxFileSize > 0 {
		body = http.MaxBytesReader(c.Writer, body, int64(h.cfg.MaxFileSize)+convertMultipartOverhead)
	}

	target := strings.TrimSuffix(h.cfg.BaseURL, "/") + convertUpstreamPrefix + path
	if q := c.Request.URL.RawQuery; q != "" {
		target += "?" + q
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, target, body)
	if err != nil {
		handleError(c, err)
		return
	}
	if c.Request.ContentLength > 0 {
		req.ContentLength = c.Request.ContentLength
	}
	if ct := c.Request.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if h.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.cfg.APIKey)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: "конвертер недоступен: домашний сервер не отвечает"})
		return
	}
	defer resp.Body.Close()

	for _, name := range []string{"Content-Type", "Content-Disposition"} {
		if v := resp.Header.Get(name); v != "" {
			c.Header(name, v)
		}
	}
	c.Header("Cache-Control", "private, no-store")
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}

func convertPathAllowed(path string) bool {
	for _, prefix := range convertAllowedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

const (
	convertTimeout            = 15 * time.Minute
	convertMultipartOverhead  = 1 << 20 // 1 МБ на границы multipart при неизвестной длине
)
