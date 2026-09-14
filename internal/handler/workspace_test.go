package handler

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"image/png"
	"mitm-departament/internal/repository"
	"mitm-departament/internal/service"
	_ "modernc.org/sqlite"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func workspaceTest(t *testing.T) (*sqlx.DB, *gin.Engine) {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, p := range []string{"../db/migration/20260325120000_init_schema.up.sql", "../db/migration/20260911120000_workspace.up.sql", "../db/migration/20260914120000_key_scans.up.sql"} {
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(b)); e != nil {
			t.Fatal(e)
		}
	}
	db.MustExec("INSERT INTO users(id,full_name,password,role,email) VALUES('a','Admin','x','admin','a@test'),('s','Staff','x','staff','s@test'),('b','Other','x','staff','b@test')")
	db.MustExec("INSERT INTO inventory(id,type,name,location,responsible_id) VALUES(1,'equipment','Scope','Lab','a')")
	db.MustExec("INSERT INTO keys(id,key_number,room_description) VALUES(1,'K1','Lab')")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	keys := service.NewKeyService(repository.NewKeyRepo(db, zap.NewNop()), repository.NewKeyLogRepo(db, zap.NewNop()), db, zap.NewNop())
	w := NewWorkspaceHandler(db, keys)
	w.RegisterPublicRoutes(r.Group("/api"), func(c *gin.Context) { c.Next() })
	private := r.Group("/api", func(c *gin.Context) {
		id := c.GetHeader("X-User")
		if id == "" {
			c.AbortWithStatus(401)
			return
		}
		c.Set(userIDKey, id)
		role := "staff"
		if id == "a" {
			role = "admin"
		}
		if header := c.GetHeader("X-Role"); header != "" {
			role = header
		}
		c.Set(roleKey, role)
	})
	w.RegisterRoutes(private)
	return db, r
}
func workspaceCall(r *gin.Engine, method, path, user, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User", user)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	return res
}
func TestWorkspacePrivateNotes(t *testing.T) {
	_, r := workspaceTest(t)
	if x := workspaceCall(r, "POST", "/notes", "s", `{"title":"Private","body":"Secret"}`); x.Code != 200 {
		t.Fatal(x.Code, x.Body.String())
	}
	if x := workspaceCall(r, "GET", "/notes", "b", ""); x.Code != 200 || x.Body.String() != "[]" {
		t.Fatal(x.Code, x.Body.String())
	}
	for _, method := range []string{"PUT", "DELETE"} {
		if x := workspaceCall(r, method, "/notes/1", "b", `{"title":"Stolen"}`); x.Code != 404 {
			t.Fatal(x.Code, x.Body.String())
		}
	}
	if x := workspaceCall(r, "GET", "/notes", "s", ""); !strings.Contains(x.Body.String(), "Secret") {
		t.Fatal(x.Body.String())
	}
	if x := workspaceCall(r, "GET", "/notes", "", ""); x.Code != 401 {
		t.Fatal(x.Code)
	}
}
func TestWorkspaceLoans(t *testing.T) {
	db, r := workspaceTest(t)
	if x := workspaceCall(r, "POST", "/inventory/1/loans", "s", `{"borrower_id":"s"}`); x.Code != 403 {
		t.Fatal(x.Code)
	}
	if x := workspaceCall(r, "POST", "/inventory/1/loans", "a", `{"borrower_id":"s"}`); x.Code != 201 {
		t.Fatal(x.Code, x.Body.String())
	}
	if x := workspaceCall(r, "POST", "/inventory/1/loans", "a", `{"borrower_id":"b"}`); x.Code != 409 {
		t.Fatal(x.Code)
	}
	if x := workspaceCall(r, "POST", "/loans/1/return", "a", ""); x.Code != 200 {
		t.Fatal(x.Code)
	}
	if x := workspaceCall(r, "POST", "/inventory/1/loans", "a", `{"borrower_id":"b"}`); x.Code != 201 {
		t.Fatal(x.Code)
	}
	var n int
	db.Get(&n, "SELECT COUNT(*) FROM inventory_loans WHERE returned_at IS NULL")
	if n != 1 {
		t.Fatal(n)
	}
}
func TestWorkspaceGuestApprovalAtomic(t *testing.T) {
	db, r := workspaceTest(t)
	x := workspaceCall(r, "POST", "/keys/1/public-link", "a", "")
	if x.Code != 200 {
		t.Fatal(x.Code, x.Body.String())
	}
	var link map[string]string
	json.Unmarshal(x.Body.Bytes(), &link)
	qr := workspaceCall(r, "GET", "/keys/1/qr", "a", "")
	if qr.Code != 200 {
		t.Fatal(qr.Code, qr.Body.String())
	}
	if _, err := png.Decode(bytes.NewReader(qr.Body.Bytes())); err != nil {
		t.Fatal(err)
	}
	if x := workspaceCall(r, "GET", "/keys/1/qr", "s", ""); x.Code != 403 {
		t.Fatal(x.Code)
	}
	path := "/public/keys/" + link["public_id"]
	for i := 0; i < 2; i++ {
		if x = workspaceCall(r, "POST", path+"/requests", "", `{"name":"Guest","affiliation":"Institute","purpose":"Visit"}`); x.Code != 201 {
			t.Fatal(x.Code, x.Body.String())
		}
	}
	if x = workspaceCall(r, "POST", "/key-requests/1/approve", "s", `{"user_id":"s"}`); x.Code != 403 {
		t.Fatal(x.Code)
	}
	if x = workspaceCall(r, "POST", "/key-requests/1/approve", "a", `{"user_id":"s"}`); x.Code != 200 {
		t.Fatal(x.Code, x.Body.String())
	}
	if x = workspaceCall(r, "POST", "/key-requests/2/approve", "a", `{"user_id":"b"}`); x.Code == 200 {
		t.Fatal("duplicate issue")
	}
	var status string
	db.Get(&status, "SELECT status FROM key_requests WHERE id=2")
	if status != "pending" {
		t.Fatal("request transaction did not roll back", status)
	}
	var n int
	db.Get(&n, "SELECT COUNT(*) FROM key_logs")
	if n != 1 {
		t.Fatal(n)
	}
	x = workspaceCall(r, "GET", path, "", "")
	if strings.Contains(x.Body.String(), "Guest") || strings.Contains(x.Body.String(), "responsible") {
		t.Fatal("guest privacy leak")
	}
}
