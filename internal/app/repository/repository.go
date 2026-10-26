package repository

import (
	"context"
	"errors"
	"time"

	"awesomeProject/internal/app/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrDraftExists = errors.New("draft exists")

type Repository struct{ DB *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{DB: db} }

func (r *Repository) GetIndexes(ctx context.Context, size int) ([]models.IndexConfiguration, error) {
	items := []models.IndexConfiguration{}
	query := r.DB.WithContext(ctx).Where("status = ?", models.Published)
	if size != 0 {
		query = query.Where("table_size = ?", size)
	}
	err := query.Preload("Likes").Order("id").Find(&items).Error
	return items, err
}

func (r *Repository) GetDraft(ctx context.Context, userID int) (models.IndexConfiguration, error) {
	var item models.IndexConfiguration
	err := r.DB.WithContext(ctx).Where("creator_id = ? AND status = ?", userID, models.Draft).Preload("Likes").First(&item).Error
	return item, err
}

func (r *Repository) GetIndex(ctx context.Context, id int, next bool) (models.IndexConfiguration, error) {
	var item models.IndexConfiguration
	query := r.DB.WithContext(ctx).Where("status = ?", models.Published)
	if id != 0 {
		query = query.Where("id = ?", id)
	}
	if err := query.Preload("Likes").Order("id").First(&item).Error; err != nil {
		return item, err
	}
	if !next {
		return item, nil
	}
	var following models.IndexConfiguration
	err := r.DB.WithContext(ctx).Where("status = ? AND id > ?", models.Published, item.ID).Preload("Likes").Order("id").First(&following).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = r.DB.WithContext(ctx).Where("status = ?", models.Published).Preload("Likes").Order("id").First(&following).Error
	}
	return following, err
}

func (r *Repository) CreateDraft(ctx context.Context, item *models.IndexConfiguration, upload func() error) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, item.CreatorID).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&models.IndexConfiguration{}).Where("creator_id = ? AND status = ?", item.CreatorID, models.Draft).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrDraftExists
		}
		if err := upload(); err != nil {
			return err
		}
		item.Status = models.Draft
		item.CreatedAt = time.Now()
		return tx.Create(item).Error
	})
}

func (r *Repository) Publish(ctx context.Context, id, userID int, description string, size int, selectivity float64) (models.IndexConfiguration, error) {
	var item models.IndexConfiguration
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.IndexConfiguration{}).Where("id = ? AND creator_id = ? AND status = ?", id, userID, models.Draft).Updates(map[string]interface{}{
			"short_description": description, "table_size": size, "selectivity": selectivity,
			"status": models.Published, "formed_at": time.Now(),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Preload("Likes").First(&item, id).Error
	})
	return item, err
}

func (r *Repository) Delete(ctx context.Context, id, userID int) error {
	result := r.DB.WithContext(ctx).Model(&models.IndexConfiguration{}).
		Where("id = ? AND creator_id = ? AND status IN ?", id, userID, []string{models.Draft, models.Published}).Update("status", models.Deleted)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) Like(ctx context.Context, id, userID, value int) (models.IndexConfiguration, error) {
	var item models.IndexConfiguration
	err := r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", id, models.Published).First(&item).Error; err != nil {
			return err
		}
		like := models.IndexConfigurationLike{UserID: userID, IndexConfigurationID: id}
		if value == 1 {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "index_configuration_id"}}, DoNothing: true}).Create(&like).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Where("user_id = ? AND index_configuration_id = ?", userID, id).Delete(&models.IndexConfigurationLike{}).Error; err != nil {
				return err
			}
		}
		return tx.Preload("Likes").First(&item, id).Error
	})
	return item, err
}

func (r *Repository) Register(ctx context.Context, user *models.User) error {
	return r.DB.WithContext(ctx).Create(user).Error
}
