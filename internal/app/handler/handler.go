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

	"awesomeProject/internal/app/models"
	"awesomeProject/internal/app/repository"
	"awesomeProject/internal/app/serializer"
	"awesomeProject/internal/app/session"
	"awesomeProject/internal/app/storage"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

type Handler struct {
	Repository *repository.Repository
	Storage    storage.Store
}

func NewHandler(repo *repository.Repository, store storage.Store) *Handler {
	return &Handler{Repository: repo, Storage: store}
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
	c.JSON(code, serializer.NewIndex(item, session.CurrentUser().ID, h.Storage.URL))
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
		result = append(result, serializer.NewIndex(item, session.CurrentUser().ID, h.Storage.URL))
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) ShowDraft(c *gin.Context) {
	item, err := h.Repository.GetDraft(c.Request.Context(), session.CurrentUser().ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, err)
		return
	}
	h.respond(c, http.StatusOK, item)
}

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
	item, err := h.Repository.Publish(c.Request.Context(), id, session.CurrentUser().ID, description, *input.TableSize, *input.Selectivity)
	if err != nil {
		fail(c, err)
		return
	}
	h.respond(c, http.StatusOK, item)
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := getID(c)
	if !ok {
		return
	}
	if err := h.Repository.Delete(c.Request.Context(), id, session.CurrentUser().ID); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusOK)
}

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
	item, err := h.Repository.Like(c.Request.Context(), id, session.CurrentUser().ID, *input.Like)
	if err != nil {
		fail(c, err)
		return
	}
	h.respond(c, http.StatusOK, item)
}

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

func (h *Handler) Authenticate(c *gin.Context) { c.Status(http.StatusNotImplemented) }
func (h *Handler) Authorize(c *gin.Context)    { c.Status(http.StatusNotImplemented) }

func (h *Handler) cleanup(keys []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, key := range keys {
		if err := h.Storage.Remove(ctx, key); err != nil {
			log.Printf("MinIO cleanup %s: %v", key, err)
		}
	}
}
