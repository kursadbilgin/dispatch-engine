package repository

import (
	"context"
	"errors"

	"github.com/kursadbilgin/dispatch-engine/internal/domain"
	"gorm.io/gorm"
)

type BatchRepository interface {
	Create(ctx context.Context, b *domain.Batch) error
	GetByID(ctx context.Context, id string) (*domain.Batch, error)
	UpdateStatus(ctx context.Context, id string, status domain.BatchStatus) error
}

type GormBatchRepo struct {
	db *gorm.DB
}

func NewGormBatchRepo(db *gorm.DB) *GormBatchRepo {
	return &GormBatchRepo{db: db}
}

func (r *GormBatchRepo) Create(ctx context.Context, b *domain.Batch) error {
	model := batchModelFromDomain(b)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	if b != nil {
		*b = *batchModelToDomain(model)
	}
	return nil
}

func (r *GormBatchRepo) GetByID(ctx context.Context, id string) (*domain.Batch, error) {
	var model BatchModel
	err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return batchModelToDomain(&model), nil
}

func (r *GormBatchRepo) UpdateStatus(ctx context.Context, id string, status domain.BatchStatus) error {
	result := r.db.WithContext(ctx).
		Model(&BatchModel{}).
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
