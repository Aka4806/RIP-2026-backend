package api

import (
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"awesomeProject/internal/app/handler"
	"awesomeProject/internal/app/media"
	"awesomeProject/internal/app/models"
	"awesomeProject/internal/app/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func StartServer(db *gorm.DB) error {
	userID := 2
	if raw := os.Getenv("CURRENT_USER_ID"); raw != "" {
		var err error
		userID, err = strconv.Atoi(raw)
		if err != nil || userID <= 0 {
			return fmt.Errorf("CURRENT_USER_ID должен быть положительным числом")
		}
	}
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		return fmt.Errorf("не удалось получить текущего пользователя %d: %w", userID, err)
	}
	address := os.Getenv("APP_ADDR")
	if address == "" {
		address = "127.0.0.1:18080"
	}

	router := NewRouter(db, userID)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	log.Printf("Откройте в браузере: http://%s/indexes", address)
	return http.Serve(listener, router)
}

func NewRouter(db *gorm.DB, userID int) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	if err := router.SetTrustedProxies(nil); err != nil {
		panic(err)
	}

	mediaBase := strings.TrimRight(os.Getenv("MEDIA_BASE_URL"), "/")
	if mediaBase == "" {
		mediaBase = "http://localhost:9000/media"
	}
	router.SetFuncMap(template.FuncMap{
		"media": media.NewResolver(mediaBase).Resolve,
		"text": func(value *string) string {
			if value == nil {
				return ""
			}
			return *value
		},
		"statusLabel": func(status string) string {
			switch status {
			case models.Draft:
				return "черновик"
			case models.Published:
				return "опубликован"
			case models.Deleted:
				return "удален"
			default:
				return "новая конфигурация"
			}
		},
		"number": formatNumber,
		"styleURL": func() string {
			const path = "/static/styles/styles.css"
			info, err := os.Stat("resources/styles/styles.css")
			if err != nil {
				return path
			}
			return path + "?v=" + strconv.FormatInt(info.ModTime().UnixNano(), 10)
		},
	})
	router.LoadHTMLGlob("templates/*.html")
	router.Static("/static", "./resources")

	repo := repository.NewRepository(db)
	handlers := handler.NewHandler(repo, userID)

	router.GET("/indexes", handlers.ListIndexes)
	router.GET("/add", handlers.ShowDraft)
	router.GET("/feed/*id", handlers.ShowFeed)
	protection := http.NewCrossOriginProtection()
	posts := router.Group("", func(c *gin.Context) {
		if err := protection.Check(c.Request); err != nil {
			handler.ShowError(c, http.StatusForbidden, "Отправка формы с другого сайта запрещена.")
			c.Abort()
			return
		}
		c.Next()
	})
	posts.POST("/indexes", handlers.CreateDraft)
	posts.POST("/indexes/:id/publish", handlers.Publish)
	posts.POST("/indexes/:id/delete", handlers.Delete)

	router.NoRoute(func(c *gin.Context) {
		if c.Request.URL.Path == "/" {
			c.Redirect(http.StatusFound, "/indexes")
			return
		}
		handler.ShowError(c, http.StatusNotFound, "Страница не найдена.")
	})
	return router
}

func formatNumber(value int) string {
	result := strconv.Itoa(value)
	for i := len(result) - 3; i > 0; i -= 3 {
		result = result[:i] + " " + result[i:]
	}
	return result
}
