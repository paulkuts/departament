package handler

import (
	"context"
	"github.com/gin-gonic/gin"
	"mitm-departament/internal/models"
	"net/http/httptest"
	"strings"
	"testing"
)

type identityUsers struct {
	UserService
	user *models.User
}

func (s *identityUsers) Create(_ context.Context, u *models.User) error        { s.user = u; return nil }
func (s *identityUsers) GetByID(context.Context, string) (*models.User, error) { return s.user, nil }

type identityAuth struct{ AuthService }

func (identityAuth) ParseToken(context.Context, string) (string, string, error) {
	return "member", "admin", nil
}

func TestRegistrationCannotChoosePrivileges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &identityUsers{}
	h := &UserHandler{userSvc: s}
	r := gin.New()
	r.POST("/register", h.Register)
	req := httptest.NewRequest("POST", "/register", strings.NewReader(`{"full_name":"Test Member","email":"Member@example.org","password":"correct-password","role":"admin","is_active":false}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 201 || s.user == nil || s.user.Role != "staff" || !s.user.IsActive || *s.user.Email != "member@example.org" {
		t.Fatalf("%d %s user=%+v", w.Code, w.Body.String(), s.user)
	}
	if strings.Contains(w.Body.String(), "correct-password") {
		t.Fatal("password exposed")
	}
}
func TestMiddlewareUsesCurrentPermissions(t *testing.T) {
	for _, active := range []bool{true, false} {
		s := &identityUsers{user: &models.User{Role: "staff", IsActive: active}}
		h := &Handler{auth: &AuthHandler{svc: identityAuth{}}, userSvc: s}
		r := gin.New()
		called := false
		r.GET("/admin", h.authMiddleware, requireRoles("admin"), func(c *gin.Context) { called = true })
		req := httptest.NewRequest("GET", "/admin", nil)
		req.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		want := 403
		if !active {
			want = 401
		}
		if w.Code != want || called {
			t.Fatalf("active=%v status=%d called=%v", active, w.Code, called)
		}
	}
}
func TestMutationGuardsPrecedeHandlers(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(roleKey, "staff") })
	g := r.Group("/api")
	(&KeyHandler{}).RegisterRoutes(g)
	(&UserHandler{}).RegisterRoutes(g)
	(&InventoryHandler{}).RegisterRoutes(g)
	(&InventoryPhotoHandler{}).RegisterRoutes(g)
	for _, x := range [][2]string{{"POST", "/keys"}, {"PUT", "/keys/1"}, {"DELETE", "/keys/1"}, {"POST", "/keys/1/lost"}, {"POST", "/users"}, {"PUT", "/users/u"}, {"DELETE", "/users/u"}, {"POST", "/users/u/activate"}, {"POST", "/inventory"}, {"PUT", "/inventory/1"}, {"DELETE", "/inventory/1"}, {"POST", "/inventory/1/photos"}, {"DELETE", "/photos/1"}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(x[0], "/api"+x[1], nil))
		if w.Code != 403 {
			t.Errorf("%v: %d", x, w.Code)
		}
	}
}

type identityArticles struct {
	ArticleService
	owner   string
	deleted bool
}

func (s *identityArticles) GetByID(context.Context, int64) (*models.Article, []models.ArticleAuthor, error) {
	return &models.Article{CreatedBy: &s.owner}, nil, nil
}
func (s *identityArticles) Delete(context.Context, int64) error { s.deleted = true; return nil }
func TestArticleOwnership(t *testing.T) {
	for _, role := range []string{"staff", "admin"} {
		for _, id := range []string{"owner", "other"} {
			s := &identityArticles{owner: "owner"}
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set(roleKey, role); c.Set(userIDKey, id) })
			NewArticleHandler(s).RegisterRoutes(r.Group(""))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("DELETE", "/articles/1", nil))
			allowed := role == "admin" || id == "owner"
			if s.deleted != allowed {
				t.Fatalf("%s %s %d", role, id, w.Code)
			}
		}
	}
}
