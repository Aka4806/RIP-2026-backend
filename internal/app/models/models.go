package models

import "time"

const (
	Draft     = "draft"
	Published = "published"
	Deleted   = "deleted"
)

type User struct {
	ID       int                      `gorm:"column:id;primaryKey"`
	Username string                   `gorm:"column:username"`
	Password string                   `gorm:"column:password;type:varchar(255);not null;unique" json:"-"`
	Likes    []IndexConfigurationLike `gorm:"foreignKey:UserID"`
}

func (User) TableName() string {
	return "users"
}

type IndexConfiguration struct {
	ID               int                      `gorm:"column:id;primaryKey"`
	Name             string                   `gorm:"column:name"`
	ShortDescription *string                  `gorm:"column:short_description"`
	Status           string                   `gorm:"column:status"`
	ImageURL         *string                  `gorm:"column:image_url"`
	VideoURL         *string                  `gorm:"column:video_url"`
	TableSize        int                      `gorm:"column:table_size"`
	Selectivity      float64                  `gorm:"column:selectivity"`
	CreatedAt        time.Time                `gorm:"column:created_at"`
	CreatorID        int                      `gorm:"column:creator_id"`
	FormedAt         *time.Time               `gorm:"column:formed_at"`
	Likes            []IndexConfigurationLike `gorm:"foreignKey:IndexConfigurationID"`
}

func (IndexConfiguration) TableName() string {
	return "index_configurations"
}

type IndexConfigurationLike struct {
	ID                   int `gorm:"column:id;primaryKey"`
	UserID               int `gorm:"column:user_id"`
	IndexConfigurationID int `gorm:"column:index_configuration_id"`
}

func (IndexConfigurationLike) TableName() string {
	return "index_configuration_likes"
}
