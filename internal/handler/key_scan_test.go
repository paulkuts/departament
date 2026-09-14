package handler

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"mitm-departament/internal/repository"
	"mitm-departament/internal/service"
	_ "modernc.org/sqlite"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// scanTest поднимает публичные ручки ключа на боевых миграциях: GET по QR-ссылке
// и POST сканирования. Вместо реального JWT сотрудник обозначается заголовком
// X-User — проверяется логика самого скана.
func scanTest(t *testing.T) (*sqlx.DB, *gin.Engine) {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, p := range []string{
		"../db/migration/20260325120000_init_schema.up.sql",
		"../db/migration/20260911120000_workspace.up.sql",
		"../db/migration/20260914120000_key_scans.up.sql",
	} {
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(b)); e != nil {
			t.Fatal(e)
		}
	}
	db.MustExec("INSERT INTO users(id,full_name,password,role,email) VALUES('s','Сотрудник Сидоров','x','staff','s@test'),('b','Второй Сотрудник','x','staff','b@test')")
	db.MustExec("INSERT INTO keys(id,key_number,room_description,status) VALUES(1,'K1','Лаборатория 305','available'),(2,'K2','Кабинет 12','lost')")
	db.MustExec("INSERT INTO key_public_links(key_id,public_id) VALUES(1,'pub-1'),(2,'pub-2')")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	keys := service.NewKeyService(repository.NewKeyRepo(db, zap.NewNop()), repository.NewKeyLogRepo(db, zap.NewNop()), db, zap.NewNop())
	w := NewWorkspaceHandler(db, keys)
	w.RegisterPublicRoutes(r.Group("/api"))
	// scan-маршрут отдельно: в тесте JWT не проверяется, сотрудник — это заголовок.
	r.POST("/api/public/keys/:public_id/scan", func(c *gin.Context) {
		if id := c.GetHeader("X-User"); id != "" {
			c.Set(userIDKey, id)
			c.Set(roleKey, "staff")
		}
		c.Next()
	}, w.ScanKey)
	private := r.Group("/api", func(c *gin.Context) {
		if id := c.GetHeader("X-User"); id != "" {
			c.Set(userIDKey, id)
			c.Set(roleKey, "admin")
		} else {
			c.AbortWithStatus(401)
		}
	})
	w.RegisterRoutes(private)
	return db, r
}

type scanCall struct {
	user  string
	body  string
	token string
}

func scanRequest(r *gin.Engine, method, path string, c scanCall) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(c.body))
	req.Header.Set("Content-Type", "application/json")
	if c.user != "" {
		req.Header.Set("X-User", c.user)
	}
	if c.token != "" {
		req.AddCookie(&http.Cookie{Name: guestCookie, Value: c.token})
	}
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	return res
}

func scanBody(t *testing.T, res *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	out := map[string]interface{}{}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatalf("ответ не разобран: %v (%s)", err, res.Body.String())
	}
	return out
}

func guestCookieValue(res *httptest.ResponseRecorder) string {
	for _, c := range res.Result().Cookies() {
		if c.Name == guestCookie {
			return c.Value
		}
	}
	return ""
}

func keyStatusInDB(t *testing.T, db *sqlx.DB, id int64) string {
	t.Helper()
	var status string
	if err := db.Get(&status, `SELECT status FROM keys WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	return status
}

// Свободный ключ: гость называет себя и получает ключ, сервер запоминает браузер.
func TestPublicScanGuestTakesKey(t *testing.T) {
	db, r := scanTest(t)

	res := scanRequest(r, "POST", "/api/public/keys/pub-1/scan", scanCall{body: `{"name":"Иван Петров","phone":"+79991234567"}`})
	if res.Code != 200 {
		t.Fatalf("код %d: %s", res.Code, res.Body.String())
	}
	out := scanBody(t, res)
	if out["action"] != "issue" || out["key_number"] != "K1" {
		t.Fatalf("ожидалась выдача K1, получено %v", out)
	}
	if token := guestCookieValue(res); token == "" {
		t.Fatal("метка браузера гостя не выдана")
	}
	if status := keyStatusInDB(t, db, 1); status != "issued" {
		t.Fatalf("ключ должен быть выдан, получено %s", status)
	}
	var userID *string
	var guest *string
	if err := db.QueryRowx(`SELECT user_id,guest_name FROM key_logs WHERE key_id=1 ORDER BY id DESC LIMIT 1`).Scan(&userID, &guest); err != nil {
		t.Fatal(err)
	}
	if userID != nil || guest == nil || *guest != "Иван Петров" {
		t.Fatalf("в журнале ожидался гость без user_id, получено %v/%v", userID, guest)
	}
}

// Повторный скан с той же меткой браузера — ключ сдан, и это видно в ответе GET.
func TestPublicScanGuestReturnsByCookie(t *testing.T) {
	db, r := scanTest(t)

	first := scanRequest(r, "POST", "/api/public/keys/pub-1/scan", scanCall{body: `{"name":"Иван Петров","phone":"+79991234567"}`})
	token := guestCookieValue(first)
	if token == "" {
		t.Fatal("метка браузера не выдана")
	}

	view := scanBody(t, scanRequest(r, "GET", "/api/public/keys/pub-1", scanCall{token: token}))
	if view["held_by_you"] != true || view["needs_guest_data"] != false || view["status"] != "issued" {
		t.Fatalf("состояние ключа для держателя неверно: %v", view)
	}

	res := scanRequest(r, "POST", "/api/public/keys/pub-1/scan", scanCall{token: token})
	if res.Code != 200 {
		t.Fatalf("код %d: %s", res.Code, res.Body.String())
	}
	if out := scanBody(t, res); out["action"] != "return" {
		t.Fatalf("ожидался возврат, получено %v", out)
	}
	if status := keyStatusInDB(t, db, 1); status != "available" {
		t.Fatalf("ключ должен быть свободен, получено %s", status)
	}

	view = scanBody(t, scanRequest(r, "GET", "/api/public/keys/pub-1", scanCall{token: token}))
	if view["held_by_you"] != false || view["status"] != "available" {
		t.Fatalf("после сдачи ключ не должен числиться за гостем: %v", view)
	}
}

// Другой гость сканирует занятый ключ — ключ переходит к нему.
func TestPublicScanTransfersBetweenGuests(t *testing.T) {
	db, r := scanTest(t)

	first := scanRequest(r, "POST", "/api/public/keys/pub-1/scan", scanCall{body: `{"name":"Иван Петров","phone":"+79991234567"}`})
	if guestCookieValue(first) == "" {
		t.Fatal("метка браузера не выдана")
	}

	res := scanRequest(r, "POST", "/api/public/keys/pub-1/scan", scanCall{body: `{"name":"Мария Сидорова","phone":"+79990001122"}`})
	if res.Code != 200 {
		t.Fatalf("код %d: %s", res.Code, res.Body.String())
	}
	out := scanBody(t, res)
	if out["action"] != "issue" || out["transferred"] != true || out["holder_name"] != "Мария Сидорова" {
		t.Fatalf("ожидалась передача Марии, получено %v", out)
	}
	if status := keyStatusInDB(t, db, 1); status != "issued" {
		t.Fatalf("ключ должен остаться выданным, получено %s", status)
	}
	var logs int
	if err := db.Get(&logs, `SELECT COUNT(*) FROM key_logs WHERE key_id=1`); err != nil {
		t.Fatal(err)
	}
	if logs != 3 { // выдача Ивану, возврат от Ивана, выдача Марии
		t.Fatalf("ожидалось 3 записи журнала, получено %d", logs)
	}
}

// Сотрудник берёт и сдаёт ключ скан-кодом, журнал пишется на его аккаунт.
func TestPublicScanStaffTakesAndReturns(t *testing.T) {
	db, r := scanTest(t)

	res := scanRequest(r, "POST", "/api/public/keys/pub-1/scan", scanCall{user: "s"})
	if res.Code != 200 {
		t.Fatalf("код %d: %s", res.Code, res.Body.String())
	}
	if out := scanBody(t, res); out["action"] != "issue" {
		t.Fatalf("ожидалась выдача, получено %v", out)
	}
	var userID *string
	if err := db.QueryRowx(`SELECT user_id FROM key_logs WHERE key_id=1 ORDER BY id DESC LIMIT 1`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if userID == nil || *userID != "s" {
		t.Fatalf("в журнале ожидался сотрудник s, получено %v", userID)
	}

	res = scanRequest(r, "POST", "/api/public/keys/pub-1/scan", scanCall{user: "s"})
	if out := scanBody(t, res); out["action"] != "return" {
		t.Fatalf("ожидался возврат, получено %v", out)
	}
	if status := keyStatusInDB(t, db, 1); status != "available" {
		t.Fatalf("ключ должен быть свободен, получено %s", status)
	}

	// Ключ, взятый гостем, сотрудник забирает себе повторным сканом.
	if res = scanRequest(r, "POST", "/api/public/keys/pub-1/scan", scanCall{body: `{"name":"Иван Петров","phone":"+79991234567"}`}); res.Code != 200 {
		t.Fatalf("гостевая выдача: %d %s", res.Code, res.Body.String())
	}
	res = scanRequest(r, "POST", "/api/public/keys/pub-1/scan", scanCall{user: "b"})
	if out := scanBody(t, res); out["action"] != "issue" || out["transferred"] != true {
		t.Fatalf("сотрудник не забрал ключ у гостя: %v", out)
	}
}

// Гость без имени и телефона получает понятную ошибку, состояние ключа не меняется.
func TestPublicScanGuestNeedsData(t *testing.T) {
	db, r := scanTest(t)

	res := scanRequest(r, "POST", "/api/public/keys/pub-1/scan", scanCall{body: `{}`})
	if res.Code != 400 {
		t.Fatalf("ожидался код 400, получено %d: %s", res.Code, res.Body.String())
	}
	if status := keyStatusInDB(t, db, 1); status != "available" {
		t.Fatalf("ключ изменился: %s", status)
	}
}

// Утерянный ключ по QR не выдаётся.
func TestPublicScanLostKey(t *testing.T) {
	_, r := scanTest(t)

	res := scanRequest(r, "POST", "/api/public/keys/pub-2/scan", scanCall{body: `{"name":"Иван Петров","phone":"+79991234567"}`})
	if res.Code != 409 || !strings.Contains(res.Body.String(), "утерянным") {
		t.Fatalf("ожидался отказ по утере, получено %d: %s", res.Code, res.Body.String())
	}
}

// Недействительная ссылка и приватность: чужие сведения по QR не раскрываются.
func TestPublicScanUnknownLinkAndPrivacy(t *testing.T) {
	_, r := scanTest(t)

	if res := scanRequest(r, "POST", "/api/public/keys/no-such/scan", scanCall{}); res.Code != 404 {
		t.Fatalf("неизвестная ссылка: код %d", res.Code)
	}
	if res := scanRequest(r, "GET", "/api/public/keys/no-such", scanCall{}); res.Code != 404 {
		t.Fatalf("неизвестная ссылка (просмотр): код %d", res.Code)
	}

	if res := scanRequest(r, "POST", "/api/public/keys/pub-1/scan", scanCall{body: `{"name":"Иван Петров","phone":"+79991234567"}`}); res.Code != 200 {
		t.Fatalf("гостевая выдача: %d", res.Code)
	}
	view := scanRequest(r, "GET", "/api/public/keys/pub-1", scanCall{})
	if view.Code != 200 {
		t.Fatalf("просмотр: %d", view.Code)
	}
	body := view.Body.String()
	for _, leak := range []string{"Иван", "Петров", "79991234567", "user_id", "guest_name", "guest_phone"} {
		if strings.Contains(body, leak) {
			t.Fatalf("ответ раскрывает сведения о держателе (%q): %s", leak, body)
		}
	}
}

// Перевыпуск QR: ссылка меняется, старая наклейка перестаёт работать.
func TestPublicLinkRenew(t *testing.T) {
	db, r := scanTest(t)

	var first string
	if err := db.Get(&first, `SELECT public_id FROM key_public_links WHERE key_id=1`); err != nil {
		t.Fatal(err)
	}
	res := scanRequest(r, "POST", "/api/keys/1/public-link", scanCall{user: "s", body: `{}`})
	if res.Code != 200 {
		t.Fatalf("повторный вызов без перевыпуска: %d %s", res.Code, res.Body.String())
	}
	if out := scanBody(t, res); out["public_id"] != first {
		t.Fatalf("ссылка изменилась без перевыпуска: %v", out)
	}

	res = scanRequest(r, "POST", "/api/keys/1/public-link", scanCall{user: "s", body: `{"renew":true}`})
	if res.Code != 200 {
		t.Fatalf("перевыпуск: %d %s", res.Code, res.Body.String())
	}
	out := scanBody(t, res)
	if out["public_id"] == first {
		t.Fatalf("ссылка не перевыпущена: %v", out)
	}
	if view := scanRequest(r, "GET", "/api/public/keys/"+first, scanCall{}); view.Code != 404 {
		t.Fatalf("старая ссылка продолжает работать: %d", view.Code)
	}
	if view := scanRequest(r, "GET", "/api/public/keys/"+out["public_id"].(string), scanCall{}); view.Code != 200 {
		t.Fatalf("новая ссылка не работает: %d %s", view.Code, view.Body.String())
	}
}
