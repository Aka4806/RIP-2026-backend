package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"awesomeProject/internal/app/handler"
	"awesomeProject/internal/app/models"
	"awesomeProject/internal/app/repository"
	"awesomeProject/internal/app/session"
	"awesomeProject/internal/app/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func StartServer(db *gorm.DB) error {
	var user models.User
	if err := db.First(&user, session.CurrentUser().ID).Error; err != nil {
		return fmt.Errorf("текущий пользователь: %w", err)
	}
	store, err := storage.New()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := store.Check(ctx); err != nil {
		return fmt.Errorf("MinIO: %w", err)
	}
	address := os.Getenv("APP_ADDR")
	if address == "" {
		address = "127.0.0.1:18083"
	}
	server := &http.Server{Addr: address, Handler: NewRouter(db, store), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 100 * time.Second, WriteTimeout: 110 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("API: http://%s/api/indexes", address)
	return server.ListenAndServe()
}

func NewRouter(db *gorm.DB, store storage.Store) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	if err := router.SetTrustedProxies(nil); err != nil {
		panic(err)
	}
	h := handler.NewHandler(repository.NewRepository(db), store)
	router.GET("/api/indexes", h.ListIndexes)
	router.GET("/api/feed/*id", h.ShowFeed)
	router.GET("/api/indexes/draft", h.ShowDraft)
	router.POST("/api/indexes", h.CreateDraft)
	router.PUT("/api/indexes/:id/publication", h.Publish)
	router.DELETE("/api/indexes/:id", h.Delete)
	router.POST("/api/indexes/:id/likes", h.Like)
	router.POST("/api/users", h.Register)
	router.POST("/api/users/authentication", h.Authenticate)
	router.POST("/api/users/authorization", h.Authorize)
	router.NoRoute(func(c *gin.Context) { c.AbortWithStatus(http.StatusNotFound) })
	return router
}
