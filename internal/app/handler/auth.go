package handler

import (
	"awesomeProject/internal/app/auth"
	"awesomeProject/internal/app/models"
	"awesomeProject/internal/app/serializer"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type Credentials struct {
	Username string `json:"username" example:"student"`
	Password string `json:"password" example:"demo-password"`
}
type TokenResponse struct {
	AccessToken string          `json:"access_token"`
	TokenType   string          `json:"token_type" example:"Bearer"`
	ExpiresAt   time.Time       `json:"expires_at"`
	User        serializer.User `json:"user"`
}
type PublicationInput struct {
	ShortDescription string  `json:"short_description" example:"B-tree for equality and range queries"`
	TableSize        int     `json:"table_size" example:"1000000"`
	Selectivity      float64 `json:"selectivity" example:"0.01"`
}
type LikeInput struct {
	Like int `json:"like" example:"1"`
}

// Authenticate godoc
// @Summary Аутентификация и получение JWT
// @Tags Пользователи
// @Accept json
// @Produce json
// @Param credentials body Credentials true "Логин и пароль"
// @Success 200 {object} TokenResponse
// @Failure 400,401,415,500 {string} string
// @Router /api/users/authentication [post]
func (h *Handler) Authenticate(c *gin.Context) {
	var input Credentials
	if !readJSON(c, &input) {
		return
	}
	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" || utf8.RuneCountInString(input.Username) > 100 || input.Password == "" || len(input.Password) > 72 {
		c.Status(http.StatusBadRequest)
		return
	}
	var user models.User
	err := h.Repository.DB.WithContext(c.Request.Context()).Where("username = ?", input.Username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.Status(http.StatusUnauthorized)
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !user.CheckPassword(input.Password) {
		c.Status(http.StatusUnauthorized)
		return
	}
	token, expires, err := h.Auth.Issue(user.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, TokenResponse{token, "Bearer", expires, serializer.User{ID: user.ID, Username: user.Username}})
}

// Deauthorize godoc
// @Summary Деавторизация и отзыв JWT через Redis blacklist
// @Tags Пользователи
// @Security BearerAuth
// @Success 200 {string} string
// @Failure 401,503 {string} string
// @Router /api/users/authorization [post]
func (h *Handler) Deauthorize(c *gin.Context) {
	value, ok := c.Get("claims")
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}
	if err := h.Auth.Revoke(c.Request.Context(), value.(*auth.Claims)); err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	c.Status(http.StatusOK)
}
