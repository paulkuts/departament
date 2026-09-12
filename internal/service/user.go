package service

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"mitm-departament/internal/config"
	"mitm-departament/internal/models"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// UserRepo — интерфейс репозитория пользователей
type UserRepo interface {
	Create(ctx context.Context, u *models.User) error
	GetByID(ctx context.Context, id string) (*models.User, error)
	ListActive(ctx context.Context, roleFilter string) ([]models.User, error)
	ListAll(ctx context.Context) ([]models.User, error)
	SetAvatar(ctx context.Context, userID, filename string) error
	Update(ctx context.Context, u *models.User) error
}

type UserService struct {
	repo UserRepo

	hasher HasherI
	cfg    config.PhotoConfig
	log    *zap.Logger
}

func NewUserService(repo UserRepo, cfg config.PhotoConfig, hasher HasherI, log *zap.Logger) *UserService {
	return &UserService{repo: repo, hasher: hasher, cfg: cfg, log: log}
}

// Create создаёт пользователя. UUID генерируется здесь.
func (s *UserService) Create(ctx context.Context, u *models.User) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}

	var err error
	u.Password, err = s.hasher.GenerateHash(u.Password)
	if err != nil {
		s.log.Error("failed to generate hash for password",
			zap.String("user_id", u.ID),
			zap.Error(err),
		)
		return fmt.Errorf("generate hash for password: %w", err)
	}

	if err := s.repo.Create(ctx, u); err != nil {
		s.log.Error("failed to create user",
			zap.String("user_id", u.ID),
			zap.Error(err),
		)
		return fmt.Errorf("create user: %w", err)
	}

	s.log.Info("user created",
		zap.String("user_id", u.ID),
		zap.String("full_name", u.FullName),
	)
	return nil
}

// GetByID возвращает пользователя по UUID
func (s *UserService) GetByID(ctx context.Context, id string) (*models.User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	if u == nil {
		return nil, fmt.Errorf("user %s not found", id)
	}
	return u, nil
}

// ListActive возвращает всех активных пользователей кроме студентов
func (s *UserService) ListActive(ctx context.Context, roleFilter string) ([]models.User, error) {
	users, err := s.repo.ListActive(ctx, roleFilter)
	if err != nil {
		return nil, fmt.Errorf("list active users: %w", err)
	}
	return users, nil
}

// ListAll возвращает всех пользователей (активных и неактивных)
func (s *UserService) ListAll(ctx context.Context) ([]models.User, error) {
	users, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all users: %w", err)
	}
	return users, nil
}

// Activate активирует пользователя
func (s *UserService) Activate(ctx context.Context, id string) error {
	u, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}

	u.IsActive = true
	return s.Update(ctx, u)
}

// Deactivate деактивирует пользователя (уволился/выпустился)
func (s *UserService) Deactivate(ctx context.Context, id string) error {
	u, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}

	u.IsActive = false
	return s.Update(ctx, u)
}

// Update обновляет данные пользователя
func (s *UserService) Update(ctx context.Context, u *models.User) error {
	if u.ID == "" {
		return fmt.Errorf("user id is required for update")
	}

	if err := s.repo.Update(ctx, u); err != nil {
		s.log.Error("failed to update user",
			zap.String("user_id", u.ID),
			zap.Error(err),
		)
		return fmt.Errorf("update user: %w", err)
	}

	s.log.Info("user updated", zap.String("user_id", u.ID))
	return nil
}

func (s *UserService) SetAvatar(ctx context.Context, userID string, file multipart.File, header *multipart.FileHeader, ext string) error {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found") // <-- Явная ошибка
	}

	// Удаляем старый файл
	if user.Avatar != nil && *user.Avatar != "" {
		_ = os.Remove(filepath.Join(s.cfg.AvatarPhotoDir, *user.Avatar))
	}

	storedName := "avatar_" + userID + ext
	dst := filepath.Join(s.cfg.AvatarPhotoDir, storedName)

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, file)
	closeErr := out.Close()

	if err != nil {
		os.Remove(dst)
		s.log.Error("failed to copy file content", zap.Error(err))
		return err
	}
	if closeErr != nil {
		os.Remove(dst)
		s.log.Error("failed to close file", zap.Error(closeErr))
		return err
	}

	return s.repo.SetAvatar(ctx, userID, storedName)
}

func (s *UserService) DeleteAvatar(ctx context.Context, userID string) error {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	if user.Avatar == nil || *user.Avatar == "" {
		return nil
	}

	if user.Avatar != nil && *user.Avatar != "" {
		_ = os.Remove(filepath.Join(s.cfg.AvatarPhotoDir, *user.Avatar))
	}
	if err := s.repo.SetAvatar(ctx, userID, ""); err != nil {
		return err
	}

	return nil
}
