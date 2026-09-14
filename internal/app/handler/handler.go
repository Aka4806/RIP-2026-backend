package handler

import (
	"net/http"
	"strconv"
	"strings"

	"awesomeProject/internal/app/repository"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	Repository *repository.Repository
}

func NewHandler(repo *repository.Repository) *Handler {
	return &Handler{Repository: repo}
}

func (h *Handler) ListIndexes(c *gin.Context) {
	selectedStep := 1
	tableSize := 0
	message := ""
	status := http.StatusOK

	if raw, exists := c.GetQuery("size_step"); exists {
		step, err := strconv.Atoi(raw)
		if err != nil || step < 0 || step > 2 {
			message = "Выберите размер таблицы на слайдере."
			status = http.StatusBadRequest
		} else {
			selectedStep = step
			switch step {
			case 0:
				tableSize = 100000
			case 1:
				tableSize = 1000000
			case 2:
				tableSize = 10000000
			}
		}
	}

	items := h.Repository.GetIndexes(tableSize)
	if message != "" {
		items = nil
	}
	likes := make(map[int]int)
	for _, item := range items {
		likes[item.ID] = len(item.Likes)
	}

	c.HTML(status, "grid.html", gin.H{
		"indexes":      items,
		"likes":        likes,
		"selectedStep": selectedStep,
		"selectedSize": tableSize,
		"error":        message,
		"active":       "grid",
	})
}

func (h *Handler) ShowDraft(c *gin.Context) {
	item, err := h.Repository.GetDraft()
	if err != nil {
		ShowError(c, http.StatusNotFound, "Черновик не найден.")
		return
	}
	c.HTML(http.StatusOK, "add.html", gin.H{
		"index":  item,
		"likes":  len(item.Likes),
		"active": "add",
	})
}

func (h *Handler) ShowFeed(c *gin.Context) {
	id := 0
	if raw := strings.TrimPrefix(c.Param("id"), "/"); raw != "" {
		var err error
		id, err = strconv.Atoi(raw)
		if err != nil || id <= 0 {
			ShowError(c, http.StatusBadRequest, "Некорректный ID конфигурации.")
			return
		}
	}

	next := c.Query("next")
	if next != "" && next != "true" && next != "false" {
		ShowError(c, http.StatusBadRequest, "Некорректный параметр перехода.")
		return
	}
	item, err := h.Repository.GetIndex(id, next == "true")
	if err != nil {
		ShowError(c, http.StatusNotFound, "Конфигурация не найдена.")
		return
	}

	c.HTML(http.StatusOK, "feed.html", gin.H{
		"index":  item,
		"likes":  len(item.Likes),
		"active": "feed",
	})
}

func ShowError(c *gin.Context, status int, message string) {
	c.HTML(status, "error.html", gin.H{
		"message": message,
		"active":  "",
	})
}
