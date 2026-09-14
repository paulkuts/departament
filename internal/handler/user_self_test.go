package handler

import (
	"context"
	"github.com/gin-gonic/gin"
	"mitm-departament/internal/models"
	"net/http/httptest"
	"strings"
	"testing"
)

type selfProfileUsers struct {
	UserService
	user    *models.User
	updated *models.User
	deleted bool
}

func (s *selfProfileUsers) GetByID(context.Context, string) (*models.User, error) { return s.user, nil }
func (s *selfProfileUsers) Update(_ context.Context, u *models.User) error {
	cp := *u
	s.updated = &cp
	return nil
}
func (s *selfProfileUsers) DeleteAvatar(context.Context, string) error { s.deleted = true; return nil }

func selfProfileRouter(user *models.User) (*selfProfileUsers, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	s := &selfProfileUsers{user: user}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(roleKey, c.GetHeader("X-Role"))
		c.Set(userIDKey, c.GetHeader("X-User"))
	})
	(&UserHandler{userSvc: s}).RegisterRoutes(r.Group(""))
	return s, r
}

func selfProfileCall(r *gin.Engine, method, path, user, role, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User", user)
	req.Header.Set("X-Role", role)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	return res
}

func member() *models.User {
	email := "s@test"
	return &models.User{ID: "s", FullName: "Сотрудник", Role: "staff", IsActive: true, Email: &email}
}

// Свой профиль правит любой вошедший; чужой — только администратор.
func TestProfileSelfEditAllowed(t *testing.T) {
	body := `{"full_name":"Новое Имя","role":"staff"}`
	for _, tc := range []struct {
		role, user, target string
		want               int
	}{
		{"staff", "s", "s", 200},
		{"teacher", "s", "s", 200},
		{"student", "s", "s", 200},
		{"admin", "s", "s", 200},
		{"admin", "a", "s", 200},
		{"staff", "s", "other", 403},
		{"student", "s", "other", 403},
	} {
		s, r := selfProfileRouter(member())
		x := selfProfileCall(r, "PUT", "/users/"+tc.target, tc.user, tc.role, body)
		if x.Code != tc.want {
			t.Fatalf("%s → %s: код %d, ожидался %d (%s)", tc.role, tc.target, x.Code, tc.want, x.Body.String())
		}
		if (x.Code == 200) != (s.updated != nil) {
			t.Fatalf("%s → %s: обновление вызвано=%v при коде %d", tc.role, tc.target, s.updated != nil, x.Code)
		}
	}
}

// Правка своего профиля не меняет роль, доступ и почту.
func TestProfileSelfEditCannotEscalate(t *testing.T) {
	s, r := selfProfileRouter(member())
	escalation := `{"full_name":"Новое Имя","role":"admin","is_active":false,"email":"hacked@example.org","office":"101"}`
	x := selfProfileCall(r, "PUT", "/users/s", "s", "staff", escalation)
	if x.Code != 200 {
		t.Fatalf("код %d: %s", x.Code, x.Body.String())
	}
	if s.updated == nil {
		t.Fatal("обновление не вызвано")
	}
	if s.updated.Role != "staff" || !s.updated.IsActive || s.updated.Email == nil || *s.updated.Email != "s@test" {
		t.Fatalf("эскалация прошла: role=%q active=%v email=%v", s.updated.Role, s.updated.IsActive, s.updated.Email)
	}
	if s.updated.FullName != "Новое Имя" || s.updated.Office == nil || *s.updated.Office != "101" {
		t.Fatalf("разрешённые поля не сохранены: %+v", s.updated)
	}
}

// Админ по-прежнему назначает роль и доступ.
func TestProfileAdminKeepsPrivileges(t *testing.T) {
	s, r := selfProfileRouter(member())
	body := `{"full_name":"Сотрудник","role":"teacher","is_active":true,"email":"new@test.org"}`
	if x := selfProfileCall(r, "PUT", "/users/s", "a", "admin", body); x.Code != 200 {
		t.Fatalf("код %d: %s", x.Code, x.Body.String())
	}
	if s.updated.Role != "teacher" || s.updated.Email == nil || *s.updated.Email != "new@test.org" {
		t.Fatalf("админ потерял права: role=%q email=%v", s.updated.Role, s.updated.Email)
	}
}

// Аватар: свой — можно, чужой — нет.
func TestProfileAvatarGuard(t *testing.T) {
	s, r := selfProfileRouter(member())
	if x := selfProfileCall(r, "DELETE", "/users/other/avatar", "s", "staff", ""); x.Code != 403 {
		t.Fatalf("чужой аватар: код %d, ожидался 403", x.Code)
	}
	if s.deleted {
		t.Fatal("чужой аватар удалён")
	}
	if x := selfProfileCall(r, "DELETE", "/users/s/avatar", "s", "staff", ""); x.Code != 200 || !s.deleted {
		t.Fatalf("свой аватар: код %d, удалён=%v", x.Code, s.deleted)
	}
}
