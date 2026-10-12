package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"awesomeProject/internal/app/models"
	"awesomeProject/internal/app/session"

	"github.com/gin-gonic/gin"
)

type upload struct {
	header      *multipart.FileHeader
	contentType string
	extension   string
}

func checkFile(header *multipart.FileHeader, image bool) (upload, bool) {
	limit := int64(50 * 1024 * 1024)
	if image {
		limit = 10 * 1024 * 1024
	}
	if header.Size <= 0 || header.Size > limit {
		return upload{}, false
	}
	file, err := header.Open()
	if err != nil {
		return upload{}, false
	}
	defer file.Close()
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return upload{}, false
	}
	buffer = buffer[:n]
	contentType := http.DetectContentType(buffer)
	ext := ""
	if image {
		switch contentType {
		case "image/png":
			ext = ".png"
		case "image/jpeg":
			ext = ".jpg"
		case "image/gif":
			ext = ".gif"
		case "image/webp":
			ext = ".webp"
		}
	} else if n >= 12 && string(buffer[4:8]) == "ftyp" {
		original := strings.ToLower(filepath.Ext(header.Filename))
		if original == ".mov" {
			ext = ".mov"
			contentType = "video/quicktime"
		}
		if original == ".mp4" {
			ext = ".mp4"
			contentType = "video/mp4"
		}
	} else if contentType == "video/webm" {
		ext = ".webm"
	}
	return upload{header: header, contentType: contentType, extension: ext}, ext != ""
}

func (h *Handler) CreateDraft(c *gin.Context) {
	kind, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || kind != "multipart/form-data" {
		c.Status(http.StatusUnsupportedMediaType)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 61*1024*1024)
	if err := c.Request.ParseMultipartForm(2 * 1024 * 1024); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	form := c.Request.MultipartForm
	defer form.RemoveAll()
	if len(form.Value) != 1 || len(form.Value["name"]) != 1 || len(form.File) != 2 || len(form.File["image"]) != 1 || len(form.File["video"]) != 1 {
		c.Status(http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(form.Value["name"][0])
	if name == "" || utf8.RuneCountInString(name) > 150 {
		c.Status(http.StatusBadRequest)
		return
	}
	image, ok := checkFile(form.File["image"][0], true)
	if !ok {
		c.Status(http.StatusBadRequest)
		return
	}
	video, ok := checkFile(form.File["video"][0], false)
	if !ok {
		c.Status(http.StatusBadRequest)
		return
	}
	item := models.IndexConfiguration{Name: name, CreatorID: session.CurrentUser().ID}
	keys := []string{}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	err = h.Repository.CreateDraft(ctx, &item, func() error {
		for _, file := range []upload{image, video} {
			token := make([]byte, 16)
			if _, err := rand.Read(token); err != nil {
				return err
			}
			key := "lab3_" + hex.EncodeToString(token) + file.extension
			source, err := file.header.Open()
			if err != nil {
				return err
			}
			keys = append(keys, key)
			err = h.Storage.Put(ctx, key, source, file.header.Size, file.contentType)
			source.Close()
			if err != nil {
				return err
			}
		}
		item.ImageURL = &keys[0]
		item.VideoURL = &keys[1]
		return nil
	})
	if err != nil {
		h.cleanup(keys)
		fail(c, err)
		return
	}
	h.respond(c, http.StatusCreated, item)
}
