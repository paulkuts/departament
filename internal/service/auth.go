package service

import (
	"context"
	"fmt"
	"mitm-departament/internal/config"
	"mitm-departament/internal/models"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"go.uber.org/zap"
)

type AuthUserRepo interface {
	GetByID(ctx context.Context, id string) (*models.User, error)
	GetCredentials(ctx context.Context, email string) (*models.User, error)
}

type TokeRepo interface {
	CreateToken(ctx context.Context, token models.Token) error
	Token(ctx context.Context, tokenID string) (models.Token, error)
	DeleteToken(ctx context.Context, tokenID string) error
}

type AuthS struct {
	userRepo  AuthUserRepo
	tokenRepo TokeRepo
	hasher    HasherI
	token     config.AuthCfg
	log       *zap.Logger
}

func NewAuthService(
	userRepo AuthUserRepo,
	tokenRepo TokeRepo,
	token config.AuthCfg,

	hasher HasherI,
	log *zap.Logger,
) *AuthS {
	return &AuthS{
		userRepo:  userRepo,
		tokenRepo: tokenRepo,
		token:     token,
		hasher:    hasher,
		log:       log,
	}
}

func (a *AuthS) SignIn(ctx context.Context, email, password string) (models.TokenOutput, error) {
	user, err := a.userRepo.GetCredentials(ctx, email)
	if err != nil {
		a.log.Debug("failed to get credentials", zap.Error(err))
		return models.TokenOutput{}, fmt.Errorf("invalid credentials")
	}

	if user == nil || !user.IsActive || a.hasher.ComparePassword(user.Password, password) != nil {
		return models.TokenOutput{}, fmt.Errorf("invalid credentials")
	}
	token, err := a.generateAndSaveTokens(ctx, user.ID, user.Role)
	if err != nil {
		a.log.Error("failed to generate or save tokens",
			zap.String("user_id", user.ID),
			zap.Error(err),
		)
		return models.TokenOutput{}, err
	}

	return token, nil
}

func (a *AuthS) Logout(ctx context.Context, tokenID string) error {
	if err := a.tokenRepo.DeleteToken(ctx, tokenID); err != nil {
		a.log.Error("failed to delete refresh token during logout",
			zap.String("token_id", tokenID),
			zap.Error(err),
		)
		return err
	}

	a.log.Info("user logged out successfully",
		zap.String("token_id", tokenID),
	)

	return nil
}

func (a *AuthS) generateAndSaveTokens(ctx context.Context, userID, role string) (models.TokenOutput, error) {
	accessToken, refreshToken, err := a.generateTokens(userID, role)
	if err != nil {
		return models.TokenOutput{}, err
	}

	if err := a.tokenRepo.CreateToken(ctx, refreshToken); err != nil {
		return models.TokenOutput{}, err
	}

	return models.TokenOutput{
		AccessToken:  accessToken,
		RefreshToken: refreshToken.TokenID,
	}, nil
}

func (a *AuthS) generateTokens(userID, role string) (string, models.Token, error) {
	accessToken, err := a.generateAccessToken(userID, role)
	if err != nil {
		return "", models.Token{}, err
	}

	refreshToken := a.generateRefreshToken(userID, role)
	return accessToken, refreshToken, nil
}

func (a *AuthS) ValidateRefreshSession(ctx context.Context, tokenID string) error {
	token, err := a.tokenRepo.Token(ctx, tokenID)
	if err != nil {
		return fmt.Errorf("session not found")
	}
	if token.ExpiresAt.Before(time.Now()) {
		_ = a.tokenRepo.DeleteToken(ctx, tokenID)
		return fmt.Errorf("session expired")
	}
	return nil
}

func (a *AuthS) generateAccessToken(userID, role string) (string, error) {
	tkn := jwt.New()

	if err := tkn.Set(jwt.SubjectKey, userID); err != nil {
		return "", fmt.Errorf("set subject: %w", err)
	}
	if err := tkn.Set("role", role); err != nil {
		return "", fmt.Errorf("set role claim: %w", err)
	}
	if err := tkn.Set(jwt.ExpirationKey, time.Now().Add(a.token.AccessTokenTTL)); err != nil {
		return "", fmt.Errorf("set exp: %w", err)
	}
	if err := tkn.Set(jwt.IssuedAtKey, time.Now()); err != nil {
		return "", fmt.Errorf("set iat: %w", err)
	}

	signed, err := jwt.Sign(tkn, jwt.WithKey(jwa.HS256, []byte(a.token.JwtSecret)))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return string(signed), nil
}

func (a *AuthS) generateRefreshToken(userID, role string) models.Token {
	return models.Token{
		UserID:    userID,
		Role:      role,
		TokenID:   uuid.New().String(),
		ExpiresAt: time.Now().Add(a.token.RefreshTokenTTL),
	}
}

func (a *AuthS) ParseToken(_ context.Context, accessToken string) (string, string, error) {
	verified, err := jwt.Parse([]byte(accessToken),
		jwt.WithKey(jwa.HS256, []byte(a.token.JwtSecret)),
		jwt.WithValidate(true),
	)
	if err != nil {
		a.log.Debug("JWT parse failed", zap.Error(err))
		return "", "", fmt.Errorf("invalid token")
	}

	userID := verified.Subject()
	a.log.Debug("JWT parsed", zap.String("sub", userID))

	if _, err := uuid.Parse(userID); err != nil {
		a.log.Debug("sub is not UUID", zap.String("sub", userID))
		return "", "", fmt.Errorf("invalid token")
	}

	roleRaw, ok := verified.Get("role")
	if !ok {
		a.log.Debug("no role claim in JWT")
		return "", "", fmt.Errorf("invalid token")
	}
	role, ok := roleRaw.(string)
	if !ok || role == "" {
		a.log.Debug("empty role claim")
		return "", "", fmt.Errorf("invalid token")
	}

	return userID, role, nil
}

func (a *AuthS) RefreshToken(ctx context.Context, tokenID string) (models.TokenOutput, error) {
	tokenDB, err := a.tokenRepo.Token(ctx, tokenID)
	if err != nil {
		return models.TokenOutput{}, fmt.Errorf("invalid session")
	}

	// Сначала проверяем, потом удаляем
	if tokenDB.ExpiresAt.Before(time.Now()) {
		_ = a.tokenRepo.DeleteToken(ctx, tokenID)
		return models.TokenOutput{}, fmt.Errorf("session expired")
	}

	user, err := a.userRepo.GetByID(ctx, tokenDB.UserID)
	if err != nil || user == nil || !user.IsActive {
		return models.TokenOutput{}, fmt.Errorf("invalid session")
	}
	// Удаляем старый (rotation)
	if err := a.tokenRepo.DeleteToken(ctx, tokenID); err != nil {
		return models.TokenOutput{}, fmt.Errorf("internal error")
	}

	token, err := a.generateAndSaveTokens(ctx, tokenDB.UserID, user.Role)
	if err != nil {
		return models.TokenOutput{}, err
	}
	return token, nil
}
