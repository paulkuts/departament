package handler

import (
	"mitm-departament/internal/models"
	"time"
)

// ========== Users ==========

type CreateUserRequest struct {
	Avatar      *string `json:"avatar"`
	FullName    string  `json:"full_name" binding:"required,min=3"`
	Password    string  `json:"password" binding:"omitempty,min=8"`
	Role        string  `json:"role" binding:"required,oneof=student teacher staff admin"`
	Position    *string `json:"position"`
	Phone       *string `json:"phone"`
	Email       *string `json:"email" binding:"omitempty,email"`
	DateOfBirth *string `json:"date_of_birth"` // формат YYYY-MM-DD
	Office      *string `json:"office"`
}

type UpdateUserRequest struct {
	Avatar      *string `json:"avatar"`
	FullName    string  `json:"full_name" binding:"required,min=3"`
	Role        string  `json:"role" binding:"required,oneof=student teacher staff admin"`
	Position    *string `json:"position"`
	Phone       *string `json:"phone"`
	Email       *string `json:"email" binding:"omitempty,email"`
	DateOfBirth *string `json:"date_of_birth"` // формат YYYY-MM-DD
	Office      *string `json:"office"`
	IsActive    *bool   `json:"is_active"`
}

type UserSignIn struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type UserResponse struct {
	ID          string     `json:"id"`
	Avatar      *string    `json:"avatar"`
	FullName    string     `json:"full_name"`
	Role        string     `json:"role"`
	Position    *string    `json:"position"`
	Phone       *string    `json:"phone,omitempty"`
	Email       *string    `json:"email,omitempty"`
	DateOfBirth *time.Time `json:"date_of_birth,omitempty"`
	Office      *string    `json:"office,omitempty"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   string     `json:"created_at"`
}

func ToUserResponse(u *models.User) UserResponse {
	var avatar *string
	if u.Avatar != nil && *u.Avatar != "" {
		a := "/api/v1/avatars/" + u.ID
		avatar = &a
	}

	return UserResponse{
		ID:          u.ID,
		Avatar:      avatar,
		FullName:    u.FullName,
		Role:        u.Role,
		Position:    u.Position,
		Phone:       u.Phone,
		Email:       u.Email,
		DateOfBirth: u.DateOfBirth,
		Office:      u.Office,
		IsActive:    u.IsActive,
		CreatedAt:   u.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

// ========== Keys ==========

type CreateKeyRequest struct {
	KeyNumber       string  `json:"key_number" binding:"required,min=1"`
	RoomDescription string  `json:"room_description" binding:"required,min=1"`
	Notes           *string `json:"notes"`
}

type UpdateKeyRequest struct {
	KeyNumber       string  `json:"key_number" binding:"required,min=1"`
	RoomDescription string  `json:"room_description" binding:"required,min=1"`
	Status          string  `json:"status" binding:"omitempty,oneof=available issued lost"`
	Notes           *string `json:"notes"`
}

type KeyResponse struct {
	ID              int64   `json:"id"`
	KeyNumber       string  `json:"key_number"`
	RoomDescription string  `json:"room_description"`
	Status          string  `json:"status"`
	Notes           *string `json:"notes,omitempty"`
}

func ToKeyResponse(k *models.Key) KeyResponse {
	return KeyResponse{
		ID:              k.ID,
		KeyNumber:       k.KeyNumber,
		RoomDescription: k.RoomDescription,
		Status:          string(k.Status),
		Notes:           k.Notes,
	}
}

// ========== Key Operations ==========

type IssueKeyRequest struct {
	UserID  string  `json:"user_id" binding:"required,uuid"`
	Comment *string `json:"comment"`
}

type ReturnKeyRequest struct {
	Comment *string `json:"comment"`
}

type KeyLogResponse struct {
	ID         int64   `json:"id"`
	KeyID      int64   `json:"key_id"`
	UserID     string  `json:"user_id"`
	ActionType string  `json:"action_type"`
	Timestamp  string  `json:"timestamp"`
	Comment    *string `json:"comment,omitempty"`
}

func ToKeyLogResponse(l *models.KeyLog) KeyLogResponse {
	return KeyLogResponse{
		ID:         l.ID,
		KeyID:      l.KeyID,
		UserID:     l.UserID,
		ActionType: string(l.ActionType),
		Timestamp:  l.Timestamp.Format("2006-01-02 15:04:05"),
		Comment:    l.Comment,
	}
}

// ========== Common ==========

type ErrorResponse struct {
	Error string `json:"error"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

// ========== Inventory ==========

type CreateInventoryRequest struct {
	Name                 string  `json:"name" binding:"required,min=1"`
	Type                 string  `json:"type" binding:"required"`
	Description          *string `json:"description"`
	Location             *string `json:"location" binding:"required,min=1"`
	Documentation        *string `json:"documentation"`
	InventoryNumber      *string `json:"inventory_number"`
	ResponsibleID        *string `json:"responsible_id" binding:"required,min=1"`
	Status               *bool   `json:"status"`
	UnavailableReason    *string `json:"unavailable_reason"`
	LastVerificationDate *string `json:"last_verification_date"`
	NextVerificationDate *string `json:"next_verification_date"`
}

type UpdateInventoryRequest struct {
	Name                 string  `json:"name" binding:"required,min=1"`
	Type                 string  `json:"type" binding:"required"`
	Description          *string `json:"description"`
	Location             *string `json:"location"`
	Documentation        *string `json:"documentation"`
	InventoryNumber      *string `json:"inventory_number"`
	ResponsibleID        *string `json:"responsible_id"`
	Status               *bool   `json:"status"`
	UnavailableReason    *string `json:"unavailable_reason"`
	LastVerificationDate *string `json:"last_verification_date"`
	NextVerificationDate *string `json:"next_verification_date"`
}

type InventoryResponse struct {
	ID                   int64   `json:"id"`
	Type                 string  `json:"type" binding:"required"`
	Name                 string  `json:"name"`
	Description          *string `json:"description,omitempty"`
	Location             *string `json:"location,omitempty"`
	Documentation        *string `json:"documentation,omitempty"`
	InventoryNumber      *string `json:"inventory_number,omitempty"`
	ResponsibleID        *string `json:"responsible_id,omitempty"`
	Status               bool    `json:"status"`
	UnavailableReason    *string `json:"unavailable_reason,omitempty"`
	LastVerificationDate *string `json:"last_verification_date,omitempty"`
	NextVerificationDate *string `json:"next_verification_date,omitempty"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

func ToInventoryResponse(e *models.Inventory) InventoryResponse {
	resp := InventoryResponse{
		ID:                e.ID,
		Type:              e.Type,
		Name:              e.Name,
		Description:       e.Description,
		Location:          e.Location,
		Documentation:     e.Documentation,
		InventoryNumber:   e.InventoryNumber,
		ResponsibleID:     e.ResponsibleID,
		Status:            e.Status,
		UnavailableReason: e.UnavailableReason,
		CreatedAt:         e.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:         e.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
	if e.LastVerificationDate != nil {
		s := e.LastVerificationDate.Format("2006-01-02")
		resp.LastVerificationDate = &s
	}
	if e.NextVerificationDate != nil {
		s := e.NextVerificationDate.Format("2006-01-02")
		resp.NextVerificationDate = &s
	}
	return resp
}

// ─── DTO ───
type ArticleAuthorDTO struct {
	UserID *string `json:"user_id"`
	Name   string  `json:"name" binding:"required,min=1"`
}

type CreateArticleRequest struct {
	Title          string             `json:"title" binding:"required,min=1"`
	Details        *string            `json:"details"`
	Indexing       *string            `json:"indexing"`
	WhiteListLevel *string            `json:"white_list_level"`
	Funding        *string            `json:"funding"`
	Link           *string            `json:"link"`
	Status         string             `json:"status" binding:"required,oneof=planned submitted published"`
	Authors        []ArticleAuthorDTO `json:"authors" binding:"required,min=1,dive"`
}

type UpdateArticleRequest = CreateArticleRequest

type ArticleAuthorResponse struct {
	ID     int64   `json:"id"`
	UserID *string `json:"user_id,omitempty"`
	Name   string  `json:"name"`
}

type ArticleResponse struct {
	ID             int64                   `json:"id"`
	Title          string                  `json:"title"`
	Details        *string                 `json:"details,omitempty"`
	Indexing       *string                 `json:"indexing,omitempty"`
	WhiteListLevel *string                 `json:"white_list_level,omitempty"`
	Funding        *string                 `json:"funding,omitempty"`
	Link           *string                 `json:"link,omitempty"`
	Status         string                  `json:"status"`
	Authors        []ArticleAuthorResponse `json:"authors"`
	CreatedBy      *string                 `json:"created_by,omitempty"`
	CreatedAt      string                  `json:"created_at"`
	UpdatedAt      string                  `json:"updated_at"`
}

// ========== Events ==========
type CreateEventRequest struct {
	CreatorID   string  `json:"creator_id" binding:"omitempty,uuid"`
	Title       *string `json:"title" binding:"required,min=1,max=255"`
	Location    *string `json:"location" binding:"required,min=1,max=500"`
	Description *string `json:"description" binding:"omitempty,max=5000"`
	StartTime   *string `json:"start_time" binding:"required"`
	IsPublic    bool    `json:"is_public"`
}

type UpdateEventRequest struct {
	Title       *string `json:"title" binding:"omitempty,min=1,max=255"`
	Location    *string `json:"location" binding:"omitempty,min=1,max=500"`
	Description *string `json:"description" binding:"omitempty,max=5000"`
	StartTime   *string `json:"start_time"`
	IsPublic    *bool   `json:"is_public"`
}

type EventResponse struct {
	ID              int64   `json:"id"`
	CreatorID       string  `json:"creator_id"`
	CreatorFullName string  `json:"creator_full_name"`
	Title           string  `json:"title"`
	Location        string  `json:"location"`
	Description     *string `json:"description,omitempty"`
	StartTime       string  `json:"start_time"`
	IsPublic        bool    `json:"is_public"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}
