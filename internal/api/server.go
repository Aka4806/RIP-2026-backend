package api

import (
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"awesomeProject/internal/app/handler"
	"awesomeProject/internal/app/repository"
	"github.com/gin-gonic/gin"
)

func StartServer() error {
	address := os.Getenv("APP_ADDR")
	if address == "" {
		address = "127.0.0.1:18080"
	}

	router := NewRouter()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	log.Printf("Откройте в браузере: http://%s/indexes", address)
	return http.Serve(listener, router)
}

func NewRouter() *gin.Engine {
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
		"media": func(key string) string {
			return mediaBase + "/" + url.PathEscape(key)
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

	repo := repository.NewRepository()
	handlers := handler.NewHandler(repo)

	router.GET("/indexes", handlers.ListIndexes)
	router.GET("/add", handlers.ShowDraft)
	router.GET("/feed/*id", handlers.ShowFeed)

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
