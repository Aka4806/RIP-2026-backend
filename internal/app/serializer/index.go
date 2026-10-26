package serializer

import "awesomeProject/internal/app/models"

type Index struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	ShortDescription string  `json:"short_description"`
	ImageURL         string  `json:"image_url"`
	VideoURL         string  `json:"video_url"`
	TableSize        int     `json:"table_size"`
	Selectivity      float64 `json:"selectivity"`
	LikesCount       int     `json:"likes_count"`
	IsOwner          int     `json:"is_owner"`
	IsLiked          int     `json:"is_liked"`
}

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

func NewIndex(item models.IndexConfiguration, userID int, mediaURL func(string) string) Index {
	result := Index{ID: item.ID, Name: item.Name, TableSize: item.TableSize, Selectivity: item.Selectivity, LikesCount: len(item.Likes)}
	if item.ShortDescription != nil {
		result.ShortDescription = *item.ShortDescription
	}
	if item.ImageURL != nil {
		result.ImageURL = mediaURL(*item.ImageURL)
	}
	if item.VideoURL != nil {
		result.VideoURL = mediaURL(*item.VideoURL)
	}
	if item.ID != 0 && item.CreatorID == userID {
		result.IsOwner = 1
	}
	for _, like := range item.Likes {
		if like.UserID == userID {
			result.IsLiked = 1
			break
		}
	}
	return result
}
