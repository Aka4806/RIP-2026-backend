package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"awesomeProject/internal/app/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SQLExecutor interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

type Repository struct {
	DB  *gorm.DB
	SQL SQLExecutor
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{DB: db, SQL: db.Statement.ConnPool}
}

func (r *Repository) GetIndexes(ctx context.Context, tableSize int) ([]models.IndexConfiguration, error) {
	var items []models.IndexConfiguration
	query := r.DB.WithContext(ctx).Where("status = ?", models.Published)
	if tableSize != 0 {
		query = query.Where("table_size = ?", tableSize)
	}
	err := query.Preload("Likes").Order("id").Find(&items).Error
	return items, err
}

func (r *Repository) GetDraft(ctx context.Context, userID int) (models.IndexConfiguration, error) {
	var item models.IndexConfiguration
	err := r.DB.WithContext(ctx).
		Where("creator_id = ? AND status = ?", userID, models.Draft).
		First(&item).Error
	return item, err
}

func (r *Repository) GetIndex(ctx context.Context, id int, next bool) (models.IndexConfiguration, error) {
	var item models.IndexConfiguration
	query := r.DB.WithContext(ctx).Where("status = ?", models.Published)
	if id == 0 {
		if err := query.Preload("Likes").Order("id").First(&item).Error; err != nil {
			return item, err
		}
	} else {
		if err := query.Preload("Likes").First(&item, id).Error; err != nil {
			return item, err
		}
	}
	if !next {
		return item, nil
	}

	var following models.IndexConfiguration
	err := r.DB.WithContext(ctx).
		Where("status = ? AND id > ?", models.Published, item.ID).
		Preload("Likes").Order("id").First(&following).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = r.DB.WithContext(ctx).Where("status = ?", models.Published).
			Preload("Likes").Order("id").First(&following).Error
	}
	return following, err
}

func (r *Repository) CreateDraft(ctx context.Context, userID int, name string) (models.IndexConfiguration, error) {
	var item models.IndexConfiguration
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			return err
		}
		err := tx.Where("creator_id = ? AND status = ?", userID, models.Draft).First(&item).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		item = models.IndexConfiguration{
			Name:      name,
			Status:    models.Draft,
			CreatorID: userID,
			CreatedAt: time.Now(),
		}
		return tx.Create(&item).Error
	})
	return item, err
}

func (r *Repository) Publish(ctx context.Context, id, userID int, description string, tableSize int, selectivity float64) error {
	result := r.DB.WithContext(ctx).Model(&models.IndexConfiguration{}).
		Where("id = ? AND creator_id = ? AND status = ?", id, userID, models.Draft).
		Updates(map[string]interface{}{
			"short_description": description,
			"table_size":        tableSize,
			"selectivity":       selectivity,
			"status":            models.Published,
			"formed_at":         time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id int) error {
	result, err := r.SQL.ExecContext(ctx,
		"UPDATE index_configurations SET status = $1 WHERE id = $2 AND status = $3",
		models.Deleted, id, models.Published,
	)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
