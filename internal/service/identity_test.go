package service

import (
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"mitm-departament/internal/config"
	"mitm-departament/internal/models"
	"mitm-departament/pkg/hasher"
	_ "modernc.org/sqlite"
	"testing"
	"time"
)

type authTestUsers struct{ user *models.User }

func (s authTestUsers) GetByID(context.Context, string) (*models.User, error) { return s.user, nil }
func (s authTestUsers) GetCredentials(context.Context, string) (*models.User, error) {
	if s.user == nil {
		return nil, fmt.Errorf("not found")
	}
	return s.user, nil
}

type authTestTokens struct{ count int }

func (s *authTestTokens) CreateToken(context.Context, models.Token) error { s.count++; return nil }
func (s *authTestTokens) Token(context.Context, string) (models.Token, error) {
	return models.Token{}, nil
}
func (s *authTestTokens) DeleteToken(context.Context, string) error { return nil }
func TestSignInChecksPasswordAndActiveState(t *testing.T) {
	h := hasher.NewHasher()
	hash, e := h.GenerateHash("valid-password")
	if e != nil {
		t.Fatal(e)
	}
	for _, x := range []struct {
		password              string
		active, exists, allow bool
	}{{"wrong", true, true, false}, {"valid-password", false, true, false}, {"valid-password", true, false, false}, {"valid-password", true, true, true}} {
		var u *models.User
		if x.exists {
			u = &models.User{ID: "user", Role: "staff", Password: hash, IsActive: x.active}
		}
		tokens := &authTestTokens{}
		svc := NewAuthService(authTestUsers{u}, tokens, config.AuthCfg{JwtSecret: "test-secret", AccessTokenTTL: time.Hour, RefreshTokenTTL: time.Hour}, h, zap.NewNop())
		out, err := svc.SignIn(context.Background(), "member@example.org", x.password)
		if (err == nil) != x.allow || (tokens.count == 1) != x.allow || (out.AccessToken != "") != x.allow {
			t.Fatalf("%+v count=%d err=%v", x, tokens.count, err)
		}
	}
}
func TestKeyReturnOwnershipIsTransactional(t *testing.T) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.MustExec(`CREATE TABLE keys(id INTEGER PRIMARY KEY,status TEXT);CREATE TABLE key_logs(id INTEGER PRIMARY KEY,key_id INTEGER,user_id TEXT,action_type TEXT,comment TEXT,timestamp DATETIME DEFAULT CURRENT_TIMESTAMP);INSERT INTO keys VALUES(1,'issued');INSERT INTO key_logs(key_id,user_id,action_type) VALUES(1,'holder','issue')`)
	svc := NewKeyService(nil, nil, db, zap.NewNop())
	if err := svc.ReturnForUser(context.Background(), 1, "other", ""); err == nil {
		t.Fatal("foreign holder returned key")
	}
	var status string
	db.Get(&status, "SELECT status FROM keys WHERE id=1")
	var count int
	db.Get(&count, "SELECT COUNT(*) FROM key_logs")
	if status != "issued" || count != 1 {
		t.Fatal("denied operation mutated state", status, count)
	}
	if err := svc.ReturnForUser(context.Background(), 1, "holder", ""); err != nil {
		t.Fatal(err)
	}
	db.Get(&status, "SELECT status FROM keys WHERE id=1")
	db.Get(&count, "SELECT COUNT(*) FROM key_logs")
	if status != "available" || count != 2 {
		t.Fatal(status, count)
	}
	if err := svc.Return(context.Background(), 1, ""); err == nil {
		t.Fatal("double return accepted")
	}
}
