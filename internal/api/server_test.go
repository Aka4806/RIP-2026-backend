package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"awesomeProject/internal/app/models"
	"awesomeProject/internal/app/serializer"
	"awesomeProject/internal/app/session"
	"awesomeProject/internal/app/storage"
	"awesomeProject/internal/database"

	"gorm.io/gorm"
)

type memoryStore struct {
	objects   map[string][]byte
	failVideo bool
}

func (s *memoryStore) Put(ctx context.Context, key string, r io.Reader, size int64, kind string) error {
	if s.failVideo && strings.HasPrefix(kind, "video/") {
		return errors.New("upload failed")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.objects[key] = data
	return nil
}
func (s *memoryStore) Remove(ctx context.Context, key string) error {
	delete(s.objects, key)
	return nil
}
func (s *memoryStore) URL(key string) string {
	if key == "" {
		return ""
	}
	return "http://localhost:9000/media/" + key
}

func multipartBody(t *testing.T, extra bool) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	if err := w.WriteField("name", "B-tree API test"); err != nil {
		t.Fatal(err)
	}
	if extra {
		if err := w.WriteField("creator_id", "1"); err != nil {
			t.Fatal(err)
		}
	}
	for field, file := range map[string]string{"image": "../../media/btree_100k.png", "video": "../../media/btree_100k.mp4"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		part, err := w.CreateFormFile(field, file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return body, w.FormDataContentType()
}

func TestAPILifecycle(t *testing.T) {
	if os.Getenv("TEST_DATABASE") != "1" {
		t.Skip("set TEST_DATABASE=1")
	}
	db, err := database.Connect()
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if err := tx.Model(&models.IndexConfiguration{}).Where("status <> ?", models.Deleted).Update("status", models.Deleted).Error; err != nil {
		t.Fatal(err)
	}
	mem := &memoryStore{objects: map[string][]byte{}}
	var store storage.Store = mem
	if os.Getenv("TEST_MINIO") == "1" {
		actual, err := storage.New()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := actual.Check(ctx); err != nil {
			t.Fatal(err)
		}
		store = actual
	}
	createdKeys := []string{}
	defer func() {
		for _, key := range createdKeys {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := store.Remove(ctx, key)
			cancel()
			if err != nil {
				t.Error(err)
			}
		}
	}()
	router := NewRouter(tx, store)
	if len(router.Routes()) != 10 {
		t.Fatalf("expected 10 methods, got %d", len(router.Routes()))
	}
	call := func(method, path, kind string, body io.Reader, status int) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, body)
		if kind != "" {
			request.Header.Set("Content-Type", kind)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != status {
			t.Fatalf("%s %s: %d, expected %d: %s", method, path, response.Code, status, response.Body.String())
		}
		if status >= 400 && response.Body.Len() != 0 {
			t.Fatal("error body must be empty")
		}
		return response
	}
	jsonCall := func(method, path, body string, status int) *httptest.ResponseRecorder {
		return call(method, path, "application/json", strings.NewReader(body), status)
	}
	card := func(response *httptest.ResponseRecorder) serializer.Index {
		t.Helper()
		var item serializer.Index
		if err := json.Unmarshal(response.Body.Bytes(), &item); err != nil {
			t.Fatal(err)
		}
		var fields map[string]interface{}
		json.Unmarshal(response.Body.Bytes(), &fields)
		if len(fields) != 10 {
			t.Fatalf("unstable fields: %v", fields)
		}
		for _, field := range []string{"status", "password", "creator_id", "created_at", "formed_at"} {
			if _, ok := fields[field]; ok {
				t.Fatalf("system field leaked: %s", field)
			}
		}
		return item
	}
	if item := card(call("GET", "/api/indexes/draft", "", nil, 200)); item.ID != 0 || item.Name != "" {
		t.Fatal("draft should be empty")
	}
	if response := call("GET", "/api/indexes", "", nil, 200); strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatal("empty list must be []")
	}
	call("GET", "/api/feed/", "", nil, 404)
	call("GET", "/indexes", "", nil, 404)
	call("GET", "/api/indexes?table_size=bad", "", nil, 400)
	jsonCall("POST", "/api/indexes", `{"name":"x"}`, 415)
	body, kind := multipartBody(t, true)
	call("POST", "/api/indexes", kind, body, 400)
	if os.Getenv("TEST_MINIO") != "1" {
		mem.failVideo = true
		body, kind = multipartBody(t, false)
		call("POST", "/api/indexes", kind, body, 500)
		if len(mem.objects) != 0 {
			t.Fatal("failed upload left objects behind")
		}
		if item := card(call("GET", "/api/indexes/draft", "", nil, 200)); item.ID != 0 {
			t.Fatal("failed upload created draft")
		}
		mem.failVideo = false
	}
	body, kind = multipartBody(t, false)
	item := card(call("POST", "/api/indexes", kind, body, 201))
	id := strconv.Itoa(item.ID)
	if item.IsOwner != 1 || item.ImageURL == "" || item.VideoURL == "" || item.ShortDescription != "" {
		t.Fatal("invalid created draft")
	}
	var stored models.IndexConfiguration
	if err := tx.First(&stored, item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.CreatorID != session.CurrentUser().ID || stored.Status != models.Draft || stored.FormedAt != nil || stored.CreatedAt.IsZero() {
		t.Fatal("invalid server fields")
	}
	for _, key := range []*string{stored.ImageURL, stored.VideoURL} {
		if key == nil || !strings.HasPrefix(*key, "lab3_") || strings.ContainsAny(*key, "/ :") {
			t.Fatal("expected generated object key")
		}
		createdKeys = append(createdKeys, *key)
		if os.Getenv("TEST_MINIO") == "1" {
			client := http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(store.URL(*key))
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("uploaded media is not accessible: %d", resp.StatusCode)
			}
		}
	}
	body, kind = multipartBody(t, false)
	call("POST", "/api/indexes", kind, body, 409)
	if draft := card(call("GET", "/api/indexes/draft", "", nil, 200)); draft.ID != item.ID {
		t.Fatal("draft mismatch")
	}
	call("GET", "/api/feed/"+id, "", nil, 404)
	foreign := models.IndexConfiguration{Name: "Foreign draft", Status: models.Draft, CreatorID: 1, CreatedAt: time.Now()}
	if err := tx.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	foreignID := strconv.Itoa(foreign.ID)
	publication := `{"short_description":"B-tree supports equality and range queries","table_size":1000000,"selectivity":0.01}`
	jsonCall("PUT", "/api/indexes/"+foreignID+"/publication", publication, 404)
	call("DELETE", "/api/indexes/"+foreignID, "", nil, 404)
	for _, invalid := range []string{`{"table_size":1}`, `{"short_description":"x","table_size":1,"selectivity":2}`, `{"short_description":"x","table_size":1,"selectivity":0,"status":"published"}`, `{"short_description":"x","table_size":1,"selectivity":null}`} {
		jsonCall("PUT", "/api/indexes/"+id+"/publication", invalid, 400)
	}
	published := card(jsonCall("PUT", "/api/indexes/"+id+"/publication", publication, 200))
	if published.TableSize != 1000000 || published.ShortDescription == "" {
		t.Fatal("publication fields lost")
	}
	jsonCall("PUT", "/api/indexes/"+id+"/publication", publication, 404)
	tx.First(&stored, item.ID)
	if stored.Status != models.Published || stored.FormedAt == nil {
		t.Fatal("publication state not saved")
	}
	card(call("GET", "/api/feed/", "", nil, 200))
	if feed := card(call("GET", "/api/feed/"+id+"?next=true", "", nil, 200)); feed.ID != item.ID {
		t.Fatal("next must wrap")
	}
	other := models.IndexConfiguration{Name: "Foreign published", Status: models.Published, CreatorID: 1, TableSize: 1000000, CreatedAt: time.Now()}
	if err := tx.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if feed := card(call("GET", "/api/feed/"+id+"?next=true", "", nil, 200)); feed.ID != other.ID {
		t.Fatal("next order")
	}
	if feed := card(call("GET", "/api/feed/"+strconv.Itoa(other.ID)+"?next=true", "", nil, 200)); feed.ID != item.ID {
		t.Fatal("wrap order")
	}
	response := call("GET", "/api/indexes?table_size=1000000", "", nil, 200)
	var list []serializer.Index
	json.Unmarshal(response.Body.Bytes(), &list)
	if len(list) != 2 || list[0].IsOwner != 1 || list[1].IsOwner != 0 {
		t.Fatal("list ownership or filter")
	}
	if response := call("GET", "/api/indexes?table_size=100000", "", nil, 200); strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatal("filter ignored")
	}
	for i := 0; i < 2; i++ {
		liked := card(jsonCall("POST", "/api/indexes/"+id+"/likes", `{"like":1}`, 200))
		if liked.LikesCount != 1 || liked.IsLiked != 1 {
			t.Fatal("like not idempotent")
		}
	}
	jsonCall("POST", "/api/indexes/"+id+"/likes", `{}`, 400)
	jsonCall("POST", "/api/indexes/"+id+"/likes", `{"like":2}`, 400)
	for i := 0; i < 2; i++ {
		unliked := card(jsonCall("POST", "/api/indexes/"+id+"/likes", `{"like":0}`, 200))
		if unliked.LikesCount != 0 || unliked.IsLiked != 0 {
			t.Fatal("unlike not idempotent")
		}
	}
	call("DELETE", "/api/indexes/"+id, "", nil, 200)
	call("GET", "/api/feed/"+id, "", nil, 404)
	call("GET", "/api/feed/"+id+"?next=true", "", nil, 404)
	jsonCall("POST", "/api/indexes/"+id+"/likes", `{"like":1}`, 404)
	jsonCall("PUT", "/api/indexes/"+id+"/publication", publication, 404)
	call("DELETE", "/api/indexes/"+id, "", nil, 404)
	if err := tx.First(&stored, item.ID).Error; err != nil || stored.Status != models.Deleted {
		t.Fatal("expected soft delete")
	}
	body, kind = multipartBody(t, false)
	newDraft := card(call("POST", "/api/indexes", kind, body, 201))
	var draftRow models.IndexConfiguration
	tx.First(&draftRow, newDraft.ID)
	createdKeys = append(createdKeys, *draftRow.ImageURL, *draftRow.VideoURL)
	call("DELETE", "/api/indexes/"+strconv.Itoa(newDraft.ID), "", nil, 200)
	username := fmt.Sprintf("api_test_%d", time.Now().UnixNano())
	registration := fmt.Sprintf(`{"username":%q,"password":"test-password"}`, username)
	registered := jsonCall("POST", "/api/users", registration, 201)
	var userDTO map[string]interface{}
	json.Unmarshal(registered.Body.Bytes(), &userDTO)
	if len(userDTO) != 2 || userDTO["username"] != username {
		t.Fatal("registration leaked fields")
	}
	var user models.User
	if err := tx.Where("username = ?", username).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if !user.CheckPassword("test-password") {
		t.Fatal("password not hashed")
	}
	if err := tx.Transaction(func(inner *gorm.DB) error {
		r := NewRouter(inner, store)
		req := httptest.NewRequest("POST", "/api/users", strings.NewReader(registration))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != 409 {
			t.Errorf("duplicate user: %d", rr.Code)
		}
		return errors.New("rollback duplicate test")
	}); err == nil {
		t.Fatal("expected rollback")
	}
	jsonCall("POST", "/api/users", `{"username":"x","password":"p","id":1}`, 400)
	call("POST", "/api/users/authentication", "", nil, 501)
	call("POST", "/api/users/authorization", "", nil, 501)
	if session.CurrentUser().ID != 2 {
		t.Fatal("singleton changed")
	}
}
