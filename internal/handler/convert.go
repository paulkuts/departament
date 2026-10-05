package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"mitm-departament/internal/config"

	"github.com/gin-gonic/gin"
)

// Конвертер файлов: тонкий прокси к движку конвертации (Transmute на CT102 в PVE,
// доступен по Tailscale). Ключ движка хранится только на сервере сайта, поэтому
// страница ходит через этот маршрут, а не в движок напрямую: тяжёлая работа идёт
// на домашнем сервере, rf-vps не нагружается.
type ConvertHandler struct {
	cfg    config.ConvertConfig
	client *http.Client

	healthMu  sync.Mutex
	healthOK  bool
	healthAt  time.Time

	retentionMu sync.Mutex
	retention   int       // срок хранения файлов на движке, минут (0 — неизвестно)
	retentionAt time.Time // когда значение получено
}

func NewConvertHandler(cfg config.ConvertConfig) *ConvertHandler {
	return &ConvertHandler{cfg: cfg, client: &http.Client{Timeout: convertTimeout}}
}

// Разрешённые разделы движка: файлы, конвертации и проверка живости.
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
	// Статус отдаём сами: страница узнаёт, настроен ли раздел, какой предел размера
	// файла, сколько файлы хранятся на движке и отвечает ли он сейчас.
	if path == "/status" {
		h.status(c)
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
		h.markHealth(false)
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: "домашний сервер с конвертером не отвечает — попробуйте позже"})
		return
	}
	defer resp.Body.Close()
	h.markHealth(true)

	for _, name := range []string{"Content-Type", "Content-Disposition"} {
		if v := resp.Header.Get(name); v != "" {
			c.Header(name, v)
		}
	}
	c.Header("Cache-Control", "private, no-store")
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}

// status — то, что страница показывает до первой конвертации и в предупреждении
// о сроке хранения файлов.
func (h *ConvertHandler) status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"enabled":           h.cfg.BaseURL != "",
		"available":         h.available(),
		"max_file_size":     h.cfg.MaxFileSize,
		"retention_minutes": h.retentionMinutes(),
	})
}

// available проверяет живость домашнего сервера не чаще, чем раз в convertHealthTTL.
func (h *ConvertHandler) available() bool {
	h.healthMu.Lock()
	defer h.healthMu.Unlock()
	if time.Since(h.healthAt) < convertHealthTTL {
		return h.healthOK
	}
	if h.cfg.BaseURL == "" {
		h.healthOK, h.healthAt = false, time.Now()
		return false
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimSuffix(h.cfg.BaseURL, "/")+convertUpstreamPrefix+"/health/ready", nil)
	if err != nil {
		h.healthOK, h.healthAt = false, time.Now()
		return false
	}
	if h.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.cfg.APIKey)
	}
	resp, err := (&http.Client{Timeout: convertProbeTimeout}).Do(req)
	h.healthAt = time.Now()
	if err != nil {
		h.healthOK = false
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	h.healthOK = resp.StatusCode == http.StatusOK
	return h.healthOK
}

func (h *ConvertHandler) markHealth(ok bool) {
	h.healthMu.Lock()
	h.healthOK, h.healthAt = ok, time.Now()
	h.healthMu.Unlock()
}

// retentionMinutes — срок хранения файлов на движке (на странице — предупреждение).
func (h *ConvertHandler) retentionMinutes() int {
	h.retentionMu.Lock()
	defer h.retentionMu.Unlock()
	if time.Since(h.retentionAt) < convertRetentionTTL {
		return h.retention
	}
	h.retentionAt = time.Now()
	h.retention = 0
	if h.cfg.BaseURL == "" {
		return h.retention
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimSuffix(h.cfg.BaseURL, "/")+convertUpstreamPrefix+"/settings", nil)
	if err != nil {
		return h.retention
	}
	if h.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.cfg.APIKey)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return h.retention
	}
	defer resp.Body.Close()
	var payload struct {
		CleanupTTL int `json:"cleanup_ttl_minutes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
		h.retention = payload.CleanupTTL
	}
	return h.retention
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
	convertTimeout           = 15 * time.Minute
	convertProbeTimeout      = 4 * time.Second
	convertHealthTTL         = 60 * time.Second
	convertRetentionTTL      = 5 * time.Minute
	convertMultipartOverhead = 1 << 20 // 1 МБ на границы multipart при неизвестной длине
)
