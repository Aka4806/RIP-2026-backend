package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"awesomeProject/internal/app/auth"
	"awesomeProject/internal/app/models"
	"awesomeProject/internal/app/repository"
	"awesomeProject/internal/app/serializer"
	"awesomeProject/internal/app/storage"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

type Handler struct {
	Repository *repository.Repository
	Storage    storage.Store
	Auth       *auth.Service
}

func NewHandler(repo *repository.Repository, store storage.Store, tokens *auth.Service) *Handler {
	return &Handler{Repository: repo, Storage: store, Auth: tokens}
}

func fail(c *gin.Context, err error) {
	var pg *pgconn.PgError
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.Status(http.StatusNotFound)
	case errors.Is(err, repository.ErrDraftExists):
		c.Status(http.StatusConflict)
	case errors.As(err, &pg) && pg.Code == "23505":
		c.Status(http.StatusConflict)
	default:
		log.Printf("API: %v", err)
		c.Status(http.StatusInternalServerError)
	}
}

func (h *Handler) respond(c *gin.Context, code int, item models.IndexConfiguration) {
	c.JSON(code, serializer.NewIndex(item, auth.UserID(c), h.Storage.URL))
}

func readJSON(c *gin.Context, dst interface{}) bool {
	kind, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || kind != "application/json" {
		c.Status(http.StatusUnsupportedMediaType)
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		c.Status(http.StatusBadRequest)
		return false
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		c.Status(http.StatusBadRequest)
		return false
	}
	return true
}

func getID(c *gin.Context) (int, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 32)
	if err != nil || id <= 0 {
		c.Status(http.StatusBadRequest)
		return 0, false
	}
	return int(id), true
}

// ListIndexes godoc
// @Summary Список опубликованных конфигураций
// @Tags Конфигурации
// @Produce json
// @Param table_size query int false "Размер таблицы"
// @Success 200 {array} serializer.Index
// @Failure 400,404,409,415,500 {string} string
// @Router /api/indexes [get]
func (h *Handler) ListIndexes(c *gin.Context) {
	size := 0
	if value, ok := c.GetQuery("table_size"); ok {
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil || parsed <= 0 {
			c.Status(http.StatusBadRequest)
			return
		}
		size = int(parsed)
	}
	items, err := h.Repository.GetIndexes(c.Request.Context(), size)
	if err != nil {
		fail(c, err)
		return
	}
	result := make([]serializer.Index, 0, len(items))
	for _, item := range items {
		result = append(result, serializer.NewIndex(item, auth.UserID(c), h.Storage.URL))
	}
	c.JSON(http.StatusOK, result)
}

// ShowDraft godoc
// @Summary Черновик текущего пользователя
// @Tags Конфигурации
// @Produce json
// @Success 200 {object} serializer.Index
// @Failure 400,404,409,415,500 {string} string
// @Security BearerAuth
// @Failure 401,503 {string} string
// @Router /api/indexes/draft [get]
func (h *Handler) ShowDraft(c *gin.Context) {
	item, err := h.Repository.GetDraft(c.Request.Context(), auth.UserID(c))
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, err)
		return
	}
	h.respond(c, http.StatusOK, item)
}

// ShowFeed godoc
// @Summary Лента конфигураций
// @Tags Конфигурации
// @Produce json
// @Param id path int true "ID карточки; для начала ленты используйте /api/feed/"
// @Param next query bool false "Следующая карточка"
// @Success 200 {object} serializer.Index
// @Failure 400,404,409,415,500 {string} string
// @Router /api/feed/{id} [get]
func (h *Handler) ShowFeed(c *gin.Context) {
	id := 0
	if raw := strings.TrimPrefix(c.Param("id"), "/"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || parsed <= 0 {
			c.Status(http.StatusBadRequest)
			return
		}
		id = int(parsed)
	}
	next := c.Query("next")
	if next != "" && next != "true" && next != "false" {
		c.Status(http.StatusBadRequest)
		return
	}
	item, err := h.Repository.GetIndex(c.Request.Context(), id, next == "true")
	if err != nil {
		fail(c, err)
		return
	}
	h.respond(c, http.StatusOK, item)
}

// Publish godoc
// @Summary Опубликовать свой черновик
// @Tags Конфигурации
// @Produce json
// @Accept json
// @Param id path int true "ID черновика"
// @Param publication body PublicationInput true "Поля публикации"
// @Success 200 {object} serializer.Index
// @Failure 400,404,409,415,500 {string} string
// @Security BearerAuth
// @Failure 401,503 {string} string
// @Router /api/indexes/{id}/publication [put]
func (h *Handler) Publish(c *gin.Context) {
	id, ok := getID(c)
	if !ok {
		return
	}
	var input struct {
		ShortDescription string   `json:"short_description"`
		TableSize        *int     `json:"table_size"`
		Selectivity      *float64 `json:"selectivity"`
	}
	if !readJSON(c, &input) {
		return
	}
	description := strings.TrimSpace(input.ShortDescription)
	if description == "" || utf8.RuneCountInString(description) > 500 || input.TableSize == nil || *input.TableSize <= 0 || *input.TableSize > math.MaxInt32 || input.Selectivity == nil || *input.Selectivity < 0 || *input.Selectivity > 1 {
		c.Status(http.StatusBadRequest)
		return
	}
	item, err := h.Repository.Publish(c.Request.Context(), id, auth.UserID(c), description, *input.TableSize, *input.Selectivity)
	if err != nil {
		fail(c, err)
		return
	}
	h.respond(c, http.StatusOK, item)
}

// Delete godoc
// @Summary Удалить свою конфигурацию
// @Tags Конфигурации
// @Produce json
// @Param id path int true "ID конфигурации"
// @Success 200 {string} string
// @Failure 400,404,409,415,500 {string} string
// @Security BearerAuth
// @Failure 401,503 {string} string
// @Router /api/indexes/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := getID(c)
	if !ok {
		return
	}
	if err := h.Repository.Delete(c.Request.Context(), id, auth.UserID(c)); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusOK)
}

// Like godoc
// @Summary Поставить или отменить лайк
// @Tags Конфигурации
// @Produce json
// @Accept json
// @Param id path int true "ID конфигурации"
// @Param like body LikeInput true "1 поставить, 0 отменить"
// @Success 200 {object} serializer.Index
// @Failure 400,404,409,415,500 {string} string
// @Security BearerAuth
// @Failure 401,503 {string} string
// @Router /api/indexes/{id}/likes [post]
func (h *Handler) Like(c *gin.Context) {
	id, ok := getID(c)
	if !ok {
		return
	}
	var input struct {
		Like *int `json:"like"`
	}
	if !readJSON(c, &input) {
		return
	}
	if input.Like == nil || (*input.Like != 0 && *input.Like != 1) {
		c.Status(http.StatusBadRequest)
		return
	}
	item, err := h.Repository.Like(c.Request.Context(), id, auth.UserID(c), *input.Like)
	if err != nil {
		fail(c, err)
		return
	}
	h.respond(c, http.StatusOK, item)
}

// Register godoc
// @Summary Регистрация пользователя
// @Tags Пользователи
// @Produce json
// @Accept json
// @Param credentials body Credentials true "Логин и пароль"
// @Success 201 {object} serializer.User
// @Failure 400,404,409,415,500 {string} string
// @Router /api/users [post]
func (h *Handler) Register(c *gin.Context) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(c, &input) {
		return
	}
	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" || utf8.RuneCountInString(input.Username) > 100 || input.Password == "" || len(input.Password) > 72 {
		c.Status(http.StatusBadRequest)
		return
	}
	user := models.User{Username: input.Username}
	if err := user.SetPassword(input.Password); err != nil {
		fail(c, err)
		return
	}
	if err := h.Repository.Register(c.Request.Context(), &user); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, serializer.User{ID: user.ID, Username: user.Username})
}

func (h *Handler) cleanup(keys []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, key := range keys {
		if err := h.Storage.Remove(ctx, key); err != nil {
			log.Printf("MinIO cleanup %s: %v", key, err)
		}
	}
}
