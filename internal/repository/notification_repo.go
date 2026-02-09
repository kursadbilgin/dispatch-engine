package repository

import (
	"context"
	"errors"
	"time"

	"github.com/kursadbilgin/dispatch-engine/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ListParams struct {
	Status   *domain.Status
	Channel  *domain.Channel
	From     *time.Time
	To       *time.Time
	Page     int
	PageSize int
}

type BatchSummary struct {
	Status domain.Status `gorm:"column:status"`
	Count  int           `gorm:"column:count"`
}

type NotificationRepository interface {
	Create(ctx context.Context, n *domain.Notification) error
	CreateBatch(ctx context.Context, notifications []*domain.Notification) error
	GetByID(ctx context.Context, id string) (*domain.Notification, error)
	GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*domain.Notification, error)
	List(ctx context.Context, params ListParams) ([]domain.Notification, int64, error)
	UpdateStatus(ctx context.Context, id string, status domain.Status) error
	UpdateStatusWithRetry(ctx context.Context, id string, status domain.Status, nextRetryAt time.Time) error
	Cancel(ctx context.Context, id string) error
	LockForSending(ctx context.Context, id string) (*domain.Notification, error)
	GetDueForRetry(ctx context.Context, limit int) ([]domain.Notification, error)
	SetProviderMessageID(ctx context.Context, id string, providerMsgID string) error
	GetBatchSummary(ctx context.Context, batchID string) ([]BatchSummary, error)
}

type GormNotificationRepo struct {
	db *gorm.DB
}

func NewGormNotificationRepo(db *gorm.DB) *GormNotificationRepo {
	return &GormNotificationRepo{db: db}
}

func (r *GormNotificationRepo) Create(ctx context.Context, n *domain.Notification) error {
	model := notificationModelFromDomain(n)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	if n != nil {
		*n = *notificationModelToDomain(model)
	}
	return nil
}

func (r *GormNotificationRepo) CreateBatch(ctx context.Context, notifications []*domain.Notification) error {
	models := make([]NotificationModel, 0, len(notifications))
	modelIndexes := make([]int, 0, len(notifications))
	for i, n := range notifications {
		model := notificationModelFromDomain(n)
		if model != nil {
			models = append(models, *model)
			modelIndexes = append(modelIndexes, i)
		}
	}

	if len(models) == 0 {
		return nil
	}

	if err := r.db.WithContext(ctx).CreateInBatches(&models, 100).Error; err != nil {
		return err
	}

	for i := range models {
		idx := modelIndexes[i]
		if idx < len(notifications) && notifications[idx] != nil {
			*notifications[idx] = *notificationModelToDomain(&models[i])
		}
	}

	return nil
}

func (r *GormNotificationRepo) GetByID(ctx context.Context, id string) (*domain.Notification, error) {
	var model NotificationModel
	err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return notificationModelToDomain(&model), nil
}

func (r *GormNotificationRepo) GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*domain.Notification, error) {
	var model NotificationModel
	err := r.db.WithContext(ctx).
		Where("idempotency_key = ?", idempotencyKey).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return notificationModelToDomain(&model), nil
}

func (r *GormNotificationRepo) List(ctx context.Context, params ListParams) ([]domain.Notification, int64, error) {
	query := r.db.WithContext(ctx).Model(&NotificationModel{})

	if params.Status != nil {
		query = query.Where("status = ?", *params.Status)
	}
	if params.Channel != nil {
		query = query.Where("channel = ?", *params.Channel)
	}
	if params.From != nil {
		query = query.Where("created_at >= ?", *params.From)
	}
	if params.To != nil {
		query = query.Where("created_at <= ?", *params.To)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := max(params.Page, 1)
	pageSize := params.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	pageSize = min(pageSize, 100)

	var models []NotificationModel
	err := query.
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&models).Error
	if err != nil {
		return nil, 0, err
	}

	notifications := make([]domain.Notification, 0, len(models))
	for i := range models {
		notifications = append(notifications, *notificationModelToDomain(&models[i]))
	}

	return notifications, total, nil
}

func (r *GormNotificationRepo) UpdateStatus(ctx context.Context, id string, status domain.Status) error {
	result := r.db.WithContext(ctx).
		Model(&NotificationModel{}).
		Where("id = ?", id).
		Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *GormNotificationRepo) UpdateStatusWithRetry(ctx context.Context, id string, status domain.Status, nextRetryAt time.Time) error {
	result := r.db.WithContext(ctx).
		Model(&NotificationModel{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":        status,
			"next_retry_at": nextRetryAt,
			"attempt_count": gorm.Expr("attempt_count + 1"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *GormNotificationRepo) Cancel(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).
		Model(&NotificationModel{}).
		Where("id = ? AND status IN ?", id, []domain.Status{domain.StatusAccepted, domain.StatusQueued}).
		Update("status", domain.StatusCanceled)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrConflict
	}
	return nil
}

func (r *GormNotificationRepo) LockForSending(ctx context.Context, id string) (*domain.Notification, error) {
	var model NotificationModel
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&model, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Skip if already in a terminal or sending state
	switch model.Status {
	case domain.StatusCanceled, domain.StatusSent, domain.StatusFailed, domain.StatusSending:
		return nil, nil
	}

	model.Status = domain.StatusSending
	if err := r.db.WithContext(ctx).
		Model(&model).
		Update("status", domain.StatusSending).Error; err != nil {
		return nil, err
	}

	return notificationModelToDomain(&model), nil
}

func (r *GormNotificationRepo) GetDueForRetry(ctx context.Context, limit int) ([]domain.Notification, error) {
	var models []NotificationModel
	err := r.db.WithContext(ctx).
		Where("status = ? AND next_retry_at <= ?", domain.StatusQueued, time.Now()).
		Order("next_retry_at ASC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, err
	}

	notifications := make([]domain.Notification, 0, len(models))
	for i := range models {
		notifications = append(notifications, *notificationModelToDomain(&models[i]))
	}

	return notifications, nil
}

func (r *GormNotificationRepo) SetProviderMessageID(ctx context.Context, id string, providerMsgID string) error {
	return r.db.WithContext(ctx).
		Model(&NotificationModel{}).
		Where("id = ?", id).
		Update("provider_message_id", providerMsgID).Error
}

func (r *GormNotificationRepo) GetBatchSummary(ctx context.Context, batchID string) ([]BatchSummary, error) {
	var summaries []BatchSummary
	err := r.db.WithContext(ctx).
		Model(&NotificationModel{}).
		Select("status, COUNT(*) as count").
		Where("batch_id = ?", batchID).
		Group("status").
		Scan(&summaries).Error
	if err != nil {
		return nil, err
	}
	return summaries, nil
}
