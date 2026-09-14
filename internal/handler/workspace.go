package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/skip2/go-qrcode"
	"mitm-departament/internal/service"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// guestCookie отмечает браузер гостя: по нему сервер узнаёт, что ключ,
// взятый по QR, сдаёт тот же человек.
const guestCookie = "key_guest"

type WorkspaceHandler struct {
	db   *sqlx.DB
	keys KeyService
}

func (h *Handler) SetWorkspace(w *WorkspaceHandler) { h.workspace = w }
func NewWorkspaceHandler(db *sqlx.DB, keys KeyService) *WorkspaceHandler {
	return &WorkspaceHandler{db: db, keys: keys}
}

// RegisterPublicRoutes: soft — мягкая аутентификация (optionalAuth), чтобы
// вошедший сотрудник видел, что ключ числится за ним, а гость оставался гостем.
func (h *WorkspaceHandler) RegisterPublicRoutes(r *gin.RouterGroup, soft gin.HandlerFunc) {
	r.GET("/public/keys/:public_id", soft, h.publicKey)
	r.POST("/public/keys/:public_id/requests", h.requestKey)
}
func (h *WorkspaceHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("/notes", h.notes)
	r.POST("/notes", h.saveNote)
	r.PUT("/notes/:id", h.saveNote)
	r.DELETE("/notes/:id", h.deleteNote)
	r.GET("/reference", h.reference)
	r.POST("/reference", requireRoles("admin", "staff"), h.saveReference)
	r.PUT("/reference/:id", requireRoles("admin", "staff"), h.saveReference)
	r.DELETE("/reference/:id", requireRoles("admin", "staff"), h.deleteReference)
	r.GET("/inventory/:id/comments", h.comments)
	r.POST("/inventory/:id/comments", h.addComment)
	r.GET("/inventory/:id/loans", h.loans)
	r.POST("/inventory/:id/loans", requireRoles("admin"), h.addLoan)
	r.POST("/loans/:id/return", requireRoles("admin"), h.returnLoan)
	r.POST("/keys/:id/public-link", requireRoles("admin"), h.publicLink)
	r.GET("/keys/:id/qr", requireRoles("admin"), h.keyQR)
	r.GET("/key-requests", requireRoles("admin"), h.requests)
	r.POST("/key-requests/:id/approve", requireRoles("admin"), h.approveRequest)
	r.POST("/key-requests/:id/reject", requireRoles("admin"), h.rejectRequest)
}
func workspaceError(c *gin.Context, code int, message string) {
	c.AbortWithStatusJSON(code, gin.H{"error": message})
}
func (h *WorkspaceHandler) list(c *gin.Context, query string, args ...interface{}) {
	rows, err := h.db.QueryxContext(c.Request.Context(), query, args...)
	if err != nil {
		workspaceError(c, 500, "Не удалось загрузить данные")
		return
	}
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		row := map[string]interface{}{}
		if err = rows.MapScan(row); err != nil {
			workspaceError(c, 500, "Не удалось загрузить данные")
			return
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		workspaceError(c, 500, "Не удалось загрузить данные")
		return
	}
	c.JSON(200, out)
}
func changed(c *gin.Context, result sql.Result, err error) bool {
	if err != nil {
		workspaceError(c, 409, "Операция недоступна: проверьте данные и состояние записи")
		return false
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		workspaceError(c, 404, "Запись не найдена или уже обработана")
		return false
	}
	return true
}
func (h *WorkspaceHandler) notes(c *gin.Context) {
	h.list(c, "SELECT id,title,body,updated_at FROM personal_notes WHERE owner_id=? ORDER BY updated_at DESC,id DESC", c.GetString(userIDKey))
}
func (h *WorkspaceHandler) saveNote(c *gin.Context) {
	var b struct {
		Title string `json:"title" binding:"required,max=250"`
		Body  string `json:"body" binding:"max=100000"`
	}
	if c.ShouldBindJSON(&b) != nil || strings.TrimSpace(b.Title) == "" {
		workspaceError(c, 400, "Укажите заголовок заметки")
		return
	}
	var res sql.Result
	var err error
	if c.Param("id") == "" {
		res, err = h.db.ExecContext(c.Request.Context(), "INSERT INTO personal_notes(owner_id,title,body) VALUES(?,?,?)", c.GetString(userIDKey), b.Title, b.Body)
	} else {
		res, err = h.db.ExecContext(c.Request.Context(), "UPDATE personal_notes SET title=?,body=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND owner_id=?", b.Title, b.Body, c.Param("id"), c.GetString(userIDKey))
	}
	if changed(c, res, err) {
		id, _ := res.LastInsertId()
		c.JSON(200, gin.H{"status": "saved", "id": id})
	}
}
func (h *WorkspaceHandler) deleteNote(c *gin.Context) {
	res, err := h.db.ExecContext(c.Request.Context(), "DELETE FROM personal_notes WHERE id=? AND owner_id=?", c.Param("id"), c.GetString(userIDKey))
	if changed(c, res, err) {
		c.Status(204)
	}
}
func (h *WorkspaceHandler) reference(c *gin.Context) {
	h.list(c, "SELECT id,category,name,value,notes FROM reference_entries ORDER BY category,name")
}
func (h *WorkspaceHandler) saveReference(c *gin.Context) {
	var b struct {
		Category string `json:"category" binding:"max=250"`
		Name     string `json:"name" binding:"required,max=250"`
		Value    string `json:"value" binding:"max=5000"`
		Notes    string `json:"notes" binding:"max=10000"`
	}
	if c.ShouldBindJSON(&b) != nil || strings.TrimSpace(b.Name) == "" {
		workspaceError(c, 400, "Укажите название")
		return
	}
	var res sql.Result
	var err error
	if c.Param("id") == "" {
		res, err = h.db.ExecContext(c.Request.Context(), "INSERT INTO reference_entries(category,name,value,notes) VALUES(?,?,?,?)", b.Category, b.Name, b.Value, b.Notes)
	} else {
		res, err = h.db.ExecContext(c.Request.Context(), "UPDATE reference_entries SET category=?,name=?,value=?,notes=? WHERE id=?", b.Category, b.Name, b.Value, b.Notes, c.Param("id"))
	}
	if changed(c, res, err) {
		id, _ := res.LastInsertId()
		c.JSON(200, gin.H{"status": "saved", "id": id})
	}
}
func (h *WorkspaceHandler) deleteReference(c *gin.Context) {
	res, err := h.db.ExecContext(c.Request.Context(), "DELETE FROM reference_entries WHERE id=?", c.Param("id"))
	if changed(c, res, err) {
		c.Status(204)
	}
}
func (h *WorkspaceHandler) comments(c *gin.Context) {
	h.list(c, "SELECT c.id,c.body,c.created_at,u.full_name AS author FROM inventory_comments c JOIN users u ON u.id=c.author_id WHERE inventory_id=? ORDER BY c.id", c.Param("id"))
}
func (h *WorkspaceHandler) addComment(c *gin.Context) {
	var b struct {
		Body string `json:"body" binding:"required,max=10000"`
	}
	if c.ShouldBindJSON(&b) != nil || strings.TrimSpace(b.Body) == "" {
		workspaceError(c, 400, "Введите комментарий")
		return
	}
	res, err := h.db.ExecContext(c.Request.Context(), "INSERT INTO inventory_comments(inventory_id,author_id,body) SELECT id,?,? FROM inventory WHERE id=?", c.GetString(userIDKey), b.Body, c.Param("id"))
	if changed(c, res, err) {
		c.JSON(201, gin.H{"status": "created"})
	}
}
func (h *WorkspaceHandler) loans(c *gin.Context) {
	h.list(c, "SELECT l.id,l.borrower_id,u.full_name AS borrower,l.comment,l.issued_at,l.returned_at FROM inventory_loans l JOIN users u ON u.id=l.borrower_id WHERE inventory_id=? ORDER BY l.id DESC", c.Param("id"))
}
func (h *WorkspaceHandler) addLoan(c *gin.Context) {
	var b struct {
		BorrowerID string `json:"borrower_id" binding:"required"`
		Comment    string `json:"comment" binding:"max=5000"`
	}
	if c.ShouldBindJSON(&b) != nil {
		workspaceError(c, 400, "Выберите сотрудника")
		return
	}
	res, err := h.db.ExecContext(c.Request.Context(), "INSERT INTO inventory_loans(inventory_id,borrower_id,issued_by,comment) SELECT i.id,u.id,?,? FROM inventory i JOIN users u ON u.id=? AND u.is_active=1 AND u.role IN ('staff','admin') WHERE i.id=? AND i.type='equipment' AND i.status=1", c.GetString(userIDKey), b.Comment, b.BorrowerID, c.Param("id"))
	if changed(c, res, err) {
		id, _ := res.LastInsertId()
		c.JSON(201, gin.H{"status": "issued", "id": id})
	}
}
func (h *WorkspaceHandler) returnLoan(c *gin.Context) {
	res, err := h.db.ExecContext(c.Request.Context(), "UPDATE inventory_loans SET returned_at=CURRENT_TIMESTAMP WHERE id=? AND returned_at IS NULL", c.Param("id"))
	if changed(c, res, err) {
		c.JSON(200, gin.H{"status": "returned"})
	}
}

// publicLink создаёт постоянную ссылку ключа. Перевыпуск (renew) заменяет её:
// старые наклейки с QR перестают работать.
func (h *WorkspaceHandler) publicLink(c *gin.Context) {
	var b struct {
		Renew bool `json:"renew"`
	}
	_ = c.ShouldBindJSON(&b)

	query := "INSERT INTO key_public_links(key_id,public_id) SELECT id,? FROM keys WHERE id=? ON CONFLICT(key_id) DO NOTHING"
	if b.Renew {
		query = "INSERT INTO key_public_links(key_id,public_id) SELECT id,? FROM keys WHERE id=? ON CONFLICT(key_id) DO UPDATE SET public_id=excluded.public_id"
	}
	_, err := h.db.ExecContext(c.Request.Context(), query, uuid.NewString(), c.Param("id"))
	if err != nil {
		workspaceError(c, 500, "Не удалось создать ссылку")
		return
	}
	var id string
	if h.db.GetContext(c.Request.Context(), &id, "SELECT public_id FROM key_public_links WHERE key_id=?", c.Param("id")) != nil {
		workspaceError(c, 404, "Ключ не найден")
		return
	}
	c.JSON(200, gin.H{"public_id": id, "path": "/public/keys/" + id})
}

// publicKey отдаёт состояние ключа по QR-ссылке. Чужих данных не раскрывает:
// видно только состояние, и держите ли ключ лично вы.
func (h *WorkspaceHandler) publicKey(c *gin.Context) {
	var k struct {
		ID        int64  `db:"id"`
		KeyNumber string `db:"key_number"`
		Room      string `db:"room_description"`
		Status    string `db:"status"`
	}
	if h.db.GetContext(c.Request.Context(), &k, "SELECT k.id,k.key_number,k.room_description,k.status FROM keys k JOIN key_public_links p ON p.key_id=k.id WHERE p.public_id=?", c.Param("public_id")) != nil {
		workspaceError(c, 404, "Ссылка недействительна. Обратитесь к администратору кафедры.")
		return
	}

	holder, err := h.keys.HolderInfo(c.Request.Context(), k.ID)
	if err != nil {
		workspaceError(c, 500, "Не удалось прочитать состояние ключа")
		return
	}
	token, _ := c.Cookie(guestCookie)
	userID := c.GetString(userIDKey)
	heldByYou := holder != nil && (userID != "" && holder.UserID != nil && *holder.UserID == userID ||
		token != "" && holder.GuestToken != nil && *holder.GuestToken == token)

	resp := gin.H{
		"key_number":       k.KeyNumber,
		"room_description": k.Room,
		"status":           k.Status,
		"needs_guest_data": userID == "" && token == "",
		"held_by_you":      heldByYou,
	}
	if heldByYou {
		resp["held_since"] = holder.Timestamp.UTC().Format(time.RFC3339)
	}
	c.JSON(200, resp)
}

// ScanKey обрабатывает сканирование QR на ключе: взять, сдать или принять ключ
// от предыдущего держателя. Сотрудник определяется по токену доступа, гость —
// по метке браузера и названному имени с телефоном.
func (h *WorkspaceHandler) ScanKey(c *gin.Context) {
	var keyID int64
	if h.db.GetContext(c.Request.Context(), &keyID, "SELECT k.id FROM keys k JOIN key_public_links p ON p.key_id=k.id WHERE p.public_id=?", c.Param("public_id")) != nil {
		workspaceError(c, 404, "Ссылка недействительна. Обратитесь к администратору кафедры.")
		return
	}

	actor := service.ScanActor{}
	var b struct {
		Name   string `json:"name" binding:"max=250"`
		Phone  string `json:"phone" binding:"max=50"`
		Intent string `json:"intent" binding:"max=10"`
	}
	_ = c.ShouldBindJSON(&b)
	if b.Intent == service.IntentTake || b.Intent == service.IntentReturn {
		actor.Intent = b.Intent
	}
	if userID := c.GetString(userIDKey); userID != "" {
		actor.UserID = userID
	} else {
		if token, err := c.Cookie(guestCookie); err == nil {
			actor.Token = token
		}
		actor.Name, actor.Phone = b.Name, b.Phone
		// Первый скан гостя: запоминаем браузер, чтобы повторный скан был сдачей.
		if actor.Token == "" && strings.TrimSpace(actor.Name) != "" && strings.TrimSpace(actor.Phone) != "" {
			token, err := newGuestToken()
			if err != nil {
				workspaceError(c, 500, "Не удалось начать работу с ключом")
				return
			}
			actor.Token = token
			http.SetCookie(c.Writer, &http.Cookie{
				Name:     guestCookie,
				Value:    token,
				Path:     "/",
				MaxAge:   365 * 24 * 60 * 60,
				Secure:   true, // сайт работает только за HTTPS
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}
	}

	out, err := h.keys.Scan(c.Request.Context(), keyID, actor)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrKeyLost):
			workspaceError(c, 409, "Ключ числится утерянным. Обратитесь к администратору кафедры.")
		case errors.Is(err, service.ErrGuestDataNeeded):
			workspaceError(c, 400, "Укажите имя и телефон")
		case errors.Is(err, service.ErrScanConflict):
			workspaceError(c, 409, "Ключ только что изменился. Отсканируйте QR ещё раз.")
		case errors.Is(err, service.ErrNotHolder):
			workspaceError(c, 409, "Ключ сейчас не у вас: возможно, его уже сдали.")
		case errors.Is(err, service.ErrAlreadyHolder):
			workspaceError(c, 409, "Ключ уже записан за вами.")
		default:
			workspaceError(c, 404, "Ключ не найден")
		}
		return
	}

	c.JSON(200, gin.H{
		"action":           string(out.Action),
		"transferred":      out.Transferred,
		"key_number":       out.KeyNumber,
		"room_description": out.Room,
		"holder_name":      out.HolderName, // ФИО гостя; сотрудник подставляет своё имя сам
	})
}

// newGuestToken — случайная метка браузера гостя.
func newGuestToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
func (h *WorkspaceHandler) keyQR(c *gin.Context) {
	var id string
	if h.db.GetContext(c.Request.Context(), &id, "SELECT public_id FROM key_public_links WHERE key_id=?", c.Param("id")) != nil {
		workspaceError(c, 404, "Сначала создайте публичную ссылку")
		return
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	// Origin is supplied by the authenticated UI to support TLS reverse proxies.
	origin := c.Query("origin")
	if origin == "" {
		origin = scheme + "://" + c.Request.Host
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host != c.Request.Host || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		workspaceError(c, 400, "Некорректный адрес сайта")
		return
	}
	png, err := qrcode.Encode(strings.TrimRight(origin, "/")+"/public/keys/"+id, qrcode.Medium, 320)
	if err != nil {
		workspaceError(c, 500, "Не удалось создать QR-код")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Data(200, "image/png", png)
}
func (h *WorkspaceHandler) requestKey(c *gin.Context) {
	var b struct {
		Name        string `json:"name" binding:"required,max=250"`
		Affiliation string `json:"affiliation" binding:"required,max=500"`
		Purpose     string `json:"purpose" binding:"required,max=2000"`
	}
	if c.ShouldBindJSON(&b) != nil || strings.TrimSpace(b.Name) == "" || strings.TrimSpace(b.Affiliation) == "" || strings.TrimSpace(b.Purpose) == "" {
		workspaceError(c, 400, "Укажите имя, организацию и цель")
		return
	}
	res, err := h.db.ExecContext(c.Request.Context(), "INSERT INTO key_requests(key_id,name,affiliation,purpose) SELECT k.id,?,?,? FROM keys k JOIN key_public_links p ON p.key_id=k.id WHERE p.public_id=? AND k.status='available'", b.Name, b.Affiliation, b.Purpose, c.Param("public_id"))
	if changed(c, res, err) {
		id, _ := res.LastInsertId()
		c.JSON(201, gin.H{"id": id, "status": "pending"})
	}
}
func (h *WorkspaceHandler) requests(c *gin.Context) {
	h.list(c, "SELECT r.*,k.key_number,k.room_description FROM key_requests r JOIN keys k ON k.id=r.key_id ORDER BY r.id DESC")
}
func (h *WorkspaceHandler) rejectRequest(c *gin.Context) {
	res, err := h.db.ExecContext(c.Request.Context(), "UPDATE key_requests SET status='rejected',reviewed_by=? WHERE id=? AND status='pending'", c.GetString(userIDKey), c.Param("id"))
	if changed(c, res, err) {
		c.JSON(200, gin.H{"status": "rejected"})
	}
}
func (h *WorkspaceHandler) approveRequest(c *gin.Context) {
	var b struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if c.ShouldBindJSON(&b) != nil {
		workspaceError(c, 400, "Выберите ответственного сотрудника")
		return
	}
	tx, err := h.db.BeginTxx(c.Request.Context(), nil)
	if err != nil {
		workspaceError(c, 500, "Не удалось начать выдачу")
		return
	}
	defer tx.Rollback()
	// First write locks the request; all key and log changes commit together.
	res, err := tx.ExecContext(c.Request.Context(), "UPDATE key_requests SET status='approved',responsible_id=?,reviewed_by=? WHERE id=? AND status='pending' AND EXISTS(SELECT 1 FROM users WHERE id=? AND is_active=1 AND role IN ('staff','admin'))", b.UserID, c.GetString(userIDKey), c.Param("id"), b.UserID)
	if !changed(c, res, err) {
		return
	}
	var keyID int64
	if tx.GetContext(c.Request.Context(), &keyID, "SELECT key_id FROM key_requests WHERE id=?", c.Param("id")) != nil {
		workspaceError(c, 500, "Не удалось прочитать заявку")
		return
	}
	res, err = tx.ExecContext(c.Request.Context(), "UPDATE keys SET status='issued' WHERE id=? AND status='available'", keyID)
	if !changed(c, res, err) {
		return
	}
	_, err = tx.ExecContext(c.Request.Context(), "INSERT INTO key_logs(key_id,user_id,action_type,comment) VALUES(?,?,'issue',?)", keyID, b.UserID, "Гостевая заявка №"+c.Param("id"))
	if err != nil {
		workspaceError(c, 409, "Не удалось выдать ключ")
		return
	}
	if tx.Commit() != nil {
		workspaceError(c, 500, "Не удалось сохранить выдачу")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "approved"})
}
