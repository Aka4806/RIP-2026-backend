package handler

import (
	"errors"
	"log"
	"math"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"awesomeProject/internal/app/models"
	"awesomeProject/internal/app/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	Repository *repository.Repository
	UserID     int
}

func NewHandler(repo *repository.Repository, userID int) *Handler {
	return &Handler{Repository: repo, UserID: userID}
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

	var items []models.IndexConfiguration
	if message == "" {
		var err error
		items, err = h.Repository.GetIndexes(c.Request.Context(), tableSize)
		if err != nil {
			databaseError(c, err)
			return
		}
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
	item, err := h.Repository.GetDraft(c.Request.Context(), h.UserID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		databaseError(c, err)
		return
	}
	h.renderDraft(c, http.StatusOK, item, "", "", "")
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
	item, err := h.Repository.GetIndex(c.Request.Context(), id, next == "true")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ShowError(c, http.StatusNotFound, "Конфигурация не найдена.")
		} else {
			databaseError(c, err)
		}
		return
	}

	c.HTML(http.StatusOK, "feed.html", gin.H{
		"index":  item,
		"likes":  len(item.Likes),
		"active": "feed",
	})
}

func databaseError(c *gin.Context, err error) {
	log.Printf("Ошибка работы с БД: %v", err)
	ShowError(c, http.StatusInternalServerError, "Не удалось загрузить данные. Попробуйте позже.")
}

func ShowError(c *gin.Context, status int, message string) {
	c.HTML(status, "error.html", gin.H{
		"message": message,
		"active":  "",
	})
}

func (h *Handler) renderDraft(c *gin.Context, status int, item models.IndexConfiguration, message, tableSize, selectivity string) {
	if item.ID != 0 && tableSize == "" && selectivity == "" && message == "" {
		tableSize = strconv.Itoa(item.TableSize)
		selectivity = strconv.FormatFloat(item.Selectivity, 'f', -1, 64)
	}
	c.HTML(status, "add.html", gin.H{
		"index":       item,
		"hasDraft":    item.ID != 0,
		"tableSize":   tableSize,
		"selectivity": selectivity,
		"error":       message,
		"active":      "add",
	})
}

func readForm(c *gin.Context) bool {
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/x-www-form-urlencoded" {
		ShowError(c, http.StatusUnsupportedMediaType, "Неподдерживаемый формат формы.")
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	if err := c.Request.ParseForm(); err != nil {
		ShowError(c, http.StatusBadRequest, "Не удалось прочитать форму.")
		return false
	}
	return true
}

func (h *Handler) CreateDraft(c *gin.Context) {
	if !readForm(c) {
		return
	}
	name := strings.TrimSpace(c.Request.PostForm.Get("name"))
	if name == "" || utf8.RuneCountInString(name) > 150 {
		h.renderDraft(c, http.StatusBadRequest, models.IndexConfiguration{Name: name}, "Введите название длиной от 1 до 150 символов.", "", "")
		return
	}
	if _, err := h.Repository.CreateDraft(c.Request.Context(), h.UserID, name); err != nil {
		databaseError(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/add")
}

func (h *Handler) Publish(c *gin.Context) {
	if !readForm(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		ShowError(c, http.StatusBadRequest, "Некорректный ID конфигурации.")
		return
	}
	item, err := h.Repository.GetDraft(c.Request.Context(), h.UserID)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && item.ID != id) {
		ShowError(c, http.StatusNotFound, "Черновик не найден.")
		return
	}
	if err != nil {
		databaseError(c, err)
		return
	}
	form := c.Request.PostForm
	description := strings.TrimSpace(form.Get("description"))
	sizeText := strings.TrimSpace(form.Get("table_size"))
	selectivityText := strings.TrimSpace(form.Get("selectivity"))
	tableSize, sizeErr := strconv.ParseInt(sizeText, 10, 32)
	selectivity, selectivityErr := strconv.ParseFloat(selectivityText, 64)
	message := ""
	switch {
	case description == "" || utf8.RuneCountInString(description) > 500:
		message = "Введите описание длиной от 1 до 500 символов."
	case sizeErr != nil || tableSize <= 0:
		message = "Размер таблицы должен быть целым числом от 1 до 2147483647."
	case selectivityErr != nil || math.IsNaN(selectivity) || math.IsInf(selectivity, 0) || selectivity < 0 || selectivity > 1:
		message = "Селективность должна быть числом от 0 до 1."
	}
	if message != "" {
		item.ShortDescription = &description
		h.renderDraft(c, http.StatusBadRequest, item, message, sizeText, selectivityText)
		return
	}
	err = h.Repository.Publish(c.Request.Context(), id, h.UserID, description, int(tableSize), selectivity)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		ShowError(c, http.StatusNotFound, "Черновик уже опубликован или удалён.")
		return
	}
	if err != nil {
		databaseError(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/feed/"+strconv.Itoa(id))
}

func (h *Handler) Delete(c *gin.Context) {
	if !readForm(c) {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		ShowError(c, http.StatusBadRequest, "Некорректный ID конфигурации.")
		return
	}
	err = h.Repository.Delete(c.Request.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		ShowError(c, http.StatusNotFound, "Опубликованная конфигурация не найдена.")
		return
	}
	if err != nil {
		databaseError(c, err)
		return
	}
	destination := "/indexes"
	if step, err := strconv.Atoi(c.Request.PostForm.Get("size_step")); err == nil && step >= 0 && step <= 2 {
		destination += "?size_step=" + strconv.Itoa(step)
	}
	c.Redirect(http.StatusSeeOther, destination)
}
