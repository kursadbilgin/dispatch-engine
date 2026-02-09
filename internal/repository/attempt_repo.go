package repository

import (
	"context"

	"github.com/kursadbilgin/dispatch-engine/internal/domain"
	"gorm.io/gorm"
)

type AttemptRepository interface {
	Create(ctx context.Context, a *domain.NotificationAttempt) error
	GetByNotificationID(ctx context.Context, notificationID string) ([]domain.NotificationAttempt, error)
}

type GormAttemptRepo struct {
	db *gorm.DB
}

func NewGormAttemptRepo(db *gorm.DB) *GormAttemptRepo {
	return &GormAttemptRepo{db: db}
}

func (r *GormAttemptRepo) Create(ctx context.Context, a *domain.NotificationAttempt) error {
	model := attemptModelFromDomain(a)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	if a != nil {
		*a = *attemptModelToDomain(model)
	}
	return nil
}

func (r *GormAttemptRepo) GetByNotificationID(ctx context.Context, notificationID string) ([]domain.NotificationAttempt, error) {
	var models []NotificationAttemptModel
	err := r.db.WithContext(ctx).
		Where("notification_id = ?", notificationID).
		Order("attempt_number ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}

	attempts := make([]domain.NotificationAttempt, 0, len(models))
	for i := range models {
		attempts = append(attempts, *attemptModelToDomain(&models[i]))
	}

	return attempts, nil
}
