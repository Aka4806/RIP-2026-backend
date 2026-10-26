package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "awesomeProject/docs"
	"awesomeProject/internal/app/auth"
	"awesomeProject/internal/app/handler"
	"awesomeProject/internal/app/repository"
	"awesomeProject/internal/app/storage"
	"github.com/redis/go-redis/v9"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func StartServer(db *gorm.DB) error {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: redisAddr, Password: os.Getenv("REDIS_PASSWORD"), DialTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, MaxRetries: 1})
	defer client.Close()
	ttl := time.Hour
	if raw := os.Getenv("JWT_TTL"); raw != "" {
		var err error
		ttl, err = time.ParseDuration(raw)
		if err != nil {
			return err
		}
	}
	tokens, err := auth.New(os.Getenv("JWT_SECRET"), ttl, auth.RedisBlacklist{Client: client})
	if err != nil {
		return err
	}
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = client.Ping(pingCtx).Err()
	pingCancel()
	if err != nil {
		return fmt.Errorf("Redis: %w", err)
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
		address = "127.0.0.1:18084"
	}
	server := &http.Server{Addr: address, Handler: NewRouter(db, store, tokens), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 100 * time.Second, WriteTimeout: 110 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("Swagger: http://%s/swagger/index.html", address)
	return server.ListenAndServe()
}

func NewRouter(db *gorm.DB, store storage.Store, tokens *auth.Service) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	if err := router.SetTrustedProxies(nil); err != nil {
		panic(err)
	}
	h := handler.NewHandler(repository.NewRepository(db), store, tokens)
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.PersistAuthorization(false)))
	public := router.Group("", tokens.Middleware(false))
	public.GET("/api/indexes", h.ListIndexes)
	public.GET("/api/feed/*id", h.ShowFeed)
	router.POST("/api/users", h.Register)
	router.POST("/api/users/authentication", h.Authenticate)
	protected := router.Group("", tokens.Middleware(true))
	protected.GET("/api/indexes/draft", h.ShowDraft)
	protected.POST("/api/indexes", h.CreateDraft)
	protected.PUT("/api/indexes/:id/publication", h.Publish)
	protected.DELETE("/api/indexes/:id", h.Delete)
	protected.POST("/api/indexes/:id/likes", h.Like)
	protected.POST("/api/users/authorization", h.Deauthorize)
	router.NoRoute(func(c *gin.Context) { c.AbortWithStatus(http.StatusNotFound) })
	return router
}
