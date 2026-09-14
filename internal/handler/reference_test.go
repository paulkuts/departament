package handler

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

// referenceCall — запрос к приватной группе с явно заданной ролью (см. workspaceTest).
func referenceCall(r *gin.Engine, method, path, user, role, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User", user)
	if role != "" {
		req.Header.Set("X-Role", role)
	}
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	return res
}

// Справочник ведут администратор и сотрудник; остальные роли только читают.
func TestWorkspaceReferenceAccess(t *testing.T) {
	db, r := workspaceTest(t)

	if x := referenceCall(r, "GET", "/reference", "", "", ""); x.Code != 401 {
		t.Fatalf("гость: %d", x.Code)
	}

	entry := `{"category":"Службы","name":"Деканат","value":"4882","notes":""}`
	if x := referenceCall(r, "POST", "/reference", "s", "staff", entry); x.Code != 200 {
		t.Fatalf("staff POST: %d %s", x.Code, x.Body.String())
	}
	if x := referenceCall(r, "PUT", "/reference/1", "s", "staff", entry); x.Code != 200 {
		t.Fatalf("staff PUT: %d %s", x.Code, x.Body.String())
	}
	if x := referenceCall(r, "POST", "/reference", "a", "admin", entry); x.Code != 200 {
		t.Fatalf("admin POST: %d %s", x.Code, x.Body.String())
	}
	if x := referenceCall(r, "DELETE", "/reference/1", "s", "staff", ""); x.Code != 204 {
		t.Fatalf("staff DELETE: %d %s", x.Code, x.Body.String())
	}

	for _, role := range []string{"teacher", "student"} {
		for _, call := range [][2]string{{"POST", "/reference"}, {"PUT", "/reference/2"}, {"DELETE", "/reference/2"}} {
			if x := referenceCall(r, call[0], call[1], "s", role, entry); x.Code != 403 {
				t.Fatalf("%s %s %s: %d", role, call[0], call[1], x.Code)
			}
		}
	}

	var n int
	if err := db.Get(&n, "SELECT COUNT(*) FROM reference_entries"); err != nil || n != 1 {
		t.Fatalf("записей в справочнике: %d (err=%v)", n, err)
	}
	if x := referenceCall(r, "GET", "/reference", "s", "student", ""); x.Code != 200 || !strings.Contains(x.Body.String(), "Деканат") {
		t.Fatalf("чтение студентом: %d %s", x.Code, x.Body.String())
	}
}
