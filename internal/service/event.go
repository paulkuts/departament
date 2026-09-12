package service

import (
	"context"
	"errors"
	"fmt"
	"mitm-departament/internal/models"
	"strings"
	"time"

	"go.uber.org/zap"
)

type EventRepo interface {
	CreateEvent(ctx context.Context, event models.Event) (int64, error)
	Event(ctx context.Context, id int64) (*models.Event, error)
	EventsList(ctx context.Context, f models.EventFilter) ([]models.Event, int64, error)
	UpdateEvent(ctx context.Context, event *models.Event, id int64) error
	DeleteEvent(ctx context.Context, eventID int64) error
}

type EventService struct {
	repo EventRepo
	log  *zap.Logger
}

func NewEventService(repo EventRepo, log *zap.Logger) *EventService {
	return &EventService{
		repo: repo,
		log:  log,
	}
}

// validateEvent проверяет корректность данных события
func (s *EventService) validateEvent(event *models.Event) error {
	if event.Title == nil {
		return errors.New("event title is required")
	}
	if len(*event.Title) > 255 {
		return errors.New("event title must not exceed 255 characters")
	}
	if event.Location == nil {
		return errors.New("event location is required")
	}
	if len(*event.Location) > 500 {
		return errors.New("event location must not exceed 500 characters")
	}
	if event.Description != nil && len(*event.Description) > 5000 {
		return errors.New("event description must not exceed 5000 characters")
	}
	if event.StartTime.IsZero() {
		return errors.New("event start time is required")
	}
	// Проверка: событие не может быть в прошлом
	if event.StartTime.Before(time.Now()) {
		return errors.New("event start time cannot be in the past")
	}
	if event.CreatorID == "" {
		return errors.New("creator ID is required")
	}
	return nil
}

// sanitizeEvent очищает входные данные от потенциально опасных символов
func (s *EventService) sanitizeEvent(event *models.Event) {
	title := strings.TrimSpace(*event.Title)
	location := strings.TrimSpace(*event.Location)
	event.Title = &title
	event.Location = &location
	if event.Description != nil {
		cleaned := strings.TrimSpace(*event.Description)
		event.Description = &cleaned
	}
}

func (s *EventService) CreateEvent(ctx context.Context, event models.Event) (int64, error) {
	// Санитизация входных данных
	s.sanitizeEvent(&event)

	// Валидация
	if err := s.validateEvent(&event); err != nil {
		s.log.Warn("invalid event data",
			zap.String("title", *event.Title),
			zap.String("creator_id", event.CreatorID),
			zap.Error(err),
		)
		return 0, err
	}

	id, err := s.repo.CreateEvent(ctx, event)
	if err != nil {
		s.log.Error("failed to create event",
			zap.String("title", *event.Title),
			zap.String("creator_id", event.CreatorID),
			zap.Error(err),
		)
		return 0, fmt.Errorf("create event: %w", err)
	}

	s.log.Info("event created",
		zap.Int64("id", id),
		zap.String("title", *event.Title),
		zap.String("creator_id", event.CreatorID),
	)
	return id, nil
}

func (s *EventService) Event(ctx context.Context, id int64) (*models.Event, error) {
	if id <= 0 {
		return nil, errors.New("invalid event ID")
	}

	event, err := s.repo.Event(ctx, id)
	if err != nil {
		s.log.Error("failed to get event",
			zap.Int64("id", id),
			zap.Error(err),
		)
		return nil, fmt.Errorf("get event: %w", err)
	}
	if event == nil {
		return nil, fmt.Errorf("event %d not found", id)
	}

	return event, nil
}

func (s *EventService) EventsList(ctx context.Context, f models.EventFilter) (models.EventList, error) {
	// Валидация и установка дефолтных значений пагинации
	if f.Paginated.Limit <= 0 {
		f.Paginated.Limit = 20
	}
	if f.Paginated.Limit > 100 {
		f.Paginated.Limit = 100 // защита от слишком больших запросов
	}
	if f.Paginated.Offset < 0 {
		f.Paginated.Offset = 0
	}

	// Валидация фильтров
	if f.Title != nil {
		cleaned := strings.TrimSpace(*f.Title)
		if cleaned == "" {
			f.Title = nil
		} else {
			f.Title = &cleaned
		}
	}
	if f.Location != nil {
		cleaned := strings.TrimSpace(*f.Location)
		if cleaned == "" {
			f.Location = nil
		} else {
			f.Location = &cleaned
		}
	}

	events, total, err := s.repo.EventsList(ctx, f)
	if err != nil {
		s.log.Error("failed to list events",
			zap.Error(err),
		)
		return models.EventList{}, fmt.Errorf("list events: %w", err)
	}

	return models.EventList{
		Events:            events,
		PaginatedMetadata: models.MakePaginatedMetadata(f.Paginated.Limit, f.Paginated.Offset, total),
	}, nil
}

// validateUpdateEvent проверяет данные для обновления
func (s *EventService) validateUpdateEvent(u *models.UpdateEvent) error {
	if u.Title != nil && *u.Title == "" {
		return errors.New("event title cannot be empty")
	}
	if u.Title != nil && len(*u.Title) > 255 {
		return errors.New("event title must not exceed 255 characters")
	}
	if u.Location != nil && *u.Location == "" {
		return errors.New("event location cannot be empty")
	}
	if u.Location != nil && len(*u.Location) > 500 {
		return errors.New("event location must not exceed 500 characters")
	}
	if u.Description != nil && len(*u.Description) > 5000 {
		return errors.New("event description must not exceed 5000 characters")
	}
	if u.StartTime != nil && u.StartTime.Before(time.Now()) {
		return errors.New("event start time cannot be in the past")
	}
	return nil
}

func (s *EventService) UpdateEvent(ctx context.Context, id int64, u *models.UpdateEvent) error {
	if id <= 0 {
		return errors.New("invalid event ID")
	}

	// Санитизация входных данных
	if u.Title != nil {
		cleaned := strings.TrimSpace(*u.Title)
		u.Title = &cleaned
	}
	if u.Location != nil {
		cleaned := strings.TrimSpace(*u.Location)
		u.Location = &cleaned
	}
	if u.Description != nil {
		cleaned := strings.TrimSpace(*u.Description)
		u.Description = &cleaned
	}

	// Валидация
	if err := s.validateUpdateEvent(u); err != nil {
		s.log.Warn("invalid update event data",
			zap.Int64("id", id),
			zap.Error(err),
		)
		return err
	}

	// Проверяем существование события перед обновлением
	existing, err := s.repo.Event(ctx, id)
	if err != nil {
		return fmt.Errorf("get event for update: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("event %d not found", id)
	}

	// Применяем обновления к существующему событию
	if u.Title != nil {
		existing.Title = u.Title
	}
	if u.Location != nil {
		existing.Location = u.Location
	}
	if u.Description != nil {
		existing.Description = u.Description
	}
	if u.StartTime != nil {
		existing.StartTime = u.StartTime
	}
	if u.IsPublic != nil {
		existing.IsPublic = *u.IsPublic
	}

	if err := s.repo.UpdateEvent(ctx, existing, id); err != nil {
		s.log.Error("failed to update event",
			zap.Int64("id", id),
			zap.Error(err),
		)
		return fmt.Errorf("update event: %w", err)
	}

	s.log.Info("event updated", zap.Int64("id", id))
	return nil
}

func (s *EventService) DeleteEvent(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid event ID")
	}

	// Проверяем существование события перед удалением
	existing, err := s.repo.Event(ctx, id)
	if err != nil {
		return fmt.Errorf("get event for delete: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("event %d not found", id)
	}

	if err := s.repo.DeleteEvent(ctx, id); err != nil {
		s.log.Error("failed to delete event",
			zap.Int64("id", id),
			zap.Error(err),
		)
		return fmt.Errorf("delete event: %w", err)
	}

	s.log.Info("event deleted", zap.Int64("id", id))
	return nil
}
