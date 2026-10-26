package api

import (
	"awesomeProject/internal/app/auth"
	"awesomeProject/internal/app/models"
	"awesomeProject/internal/database"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type testBlacklist struct{ entries map[string]bool }

func (b *testBlacklist) Contains(_ context.Context, id string) (bool, error) {
	return b.entries[id], nil
}
func (b *testBlacklist) Add(_ context.Context, id string, _ int, _ time.Duration) error {
	b.entries[id] = true
	return nil
}
func TestAuthenticationLifecycle(t *testing.T) {
	if os.Getenv("TEST_DATABASE") != "1" {
		t.Skip("set TEST_DATABASE=1")
	}
	db, err := database.Connect()
	if err != nil {
		t.Fatal(err)
	}
	sql, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sql.Close()
	tx := db.Begin()
	defer tx.Rollback()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	tokens, err := auth.New(strings.Repeat("x", 32), time.Hour, &testBlacklist{entries: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(tx, &memoryStore{objects: map[string][]byte{}}, tokens)
	call := func(method, path, body, token string, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != status {
			t.Fatalf("%s %s got %d want %d: %s", method, path, rr.Code, status, rr.Body.String())
		}
		return rr
	}
	username := fmt.Sprintf("auth_%d", time.Now().UnixNano())
	credentials := fmt.Sprintf(`{"username":%q,"password":"demo-password"}`, username)
	call("POST", "/api/users", credentials, "", 201)
	call("POST", "/api/users/authentication", fmt.Sprintf(`{"username":%q,"password":"wrong"}`, username), "", 401)
	call("POST", "/api/users/authentication", `{"username":"does-not-exist","password":"wrong"}`, "", 401)
	rr := call("POST", "/api/users/authentication", credentials, "", 200)
	var response struct {
		AccessToken string `json:"access_token"`
		User        struct {
			ID int `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	token := response.AccessToken
	if token == "" {
		t.Fatal("missing JWT")
	}
	call("GET", "/api/indexes/draft", "", token, 200)
	call("POST", "/api/users/authorization", "", "", 401)
	call("GET", "/api/indexes", "", "broken", 200)
	call("GET", "/api/indexes", "", "", 200)
	for _, entry := range [][2]string{{"GET", "/api/indexes/draft"}, {"POST", "/api/indexes"}, {"PUT", "/api/indexes/1/publication"}, {"DELETE", "/api/indexes/1"}, {"POST", "/api/indexes/1/likes"}} {
		call(entry[0], entry[1], "", "", 401)
		call(entry[0], entry[1], "", "broken", 401)
	}
	body, kind := multipartBody(t, false)
	req := httptest.NewRequest("POST", "/api/indexes", body)
	req.Header.Set("Content-Type", kind)
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	var card struct {
		ID int `json:"id"`
	}
	json.Unmarshal(rr.Body.Bytes(), &card)
	var stored models.IndexConfiguration
	tx.First(&stored, card.ID)
	if stored.CreatorID != response.User.ID {
		t.Fatal("creator not taken from JWT")
	}
	publication := `{"short_description":"JWT publication","table_size":100000,"selectivity":0.01}`
	url := fmt.Sprintf("/api/indexes/%d/publication", card.ID)
	call("PUT", url, publication, "", 401)
	foreign, _, _ := tokens.Issue(1)
	call("PUT", url, publication, foreign, 404)
	call("PUT", url, publication, token, 200)
	tx.First(&stored, card.ID)
	if stored.Status != models.Published || stored.TableSize != 100000 {
		t.Fatal("publication not persisted")
	}
	call("POST", "/api/users/authorization", "", token, 200)
	call("POST", "/api/users/authorization", "", token, 401)
	call("GET", "/api/indexes", "", token, 200)
	fresh := call("POST", "/api/users/authentication", credentials, "", 200)
	json.Unmarshal(fresh.Body.Bytes(), &response)
	if response.AccessToken == token {
		t.Fatal("token ID not unique")
	}
	call("GET", "/api/indexes/draft", "", response.AccessToken, 200)
	call("GET", "/swagger/index.html", "", "", 200)
	spec := call("GET", "/swagger/doc.json", "", "", 200)
	var doc map[string]interface{}
	if json.Unmarshal(spec.Body.Bytes(), &doc) != nil {
		t.Fatal("invalid swagger")
	}
}

// Business routes and Swagger expose exactly the same ten methods as LR3.
func TestAPIRoutesAndSwaggerMatchLR3(t *testing.T) {
	tokens, err := auth.New(strings.Repeat("x", 32), time.Hour, &testBlacklist{entries: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(nil, &memoryStore{objects: map[string][]byte{}}, tokens)
	expected := map[string]bool{
		"GET /api/indexes": true, "GET /api/feed/*id": true, "GET /api/indexes/draft": true,
		"POST /api/indexes": true, "PUT /api/indexes/:id/publication": true, "DELETE /api/indexes/:id": true,
		"POST /api/indexes/:id/likes": true, "POST /api/users": true,
		"POST /api/users/authentication": true, "POST /api/users/authorization": true,
	}
	count := 0
	for _, route := range router.Routes() {
		if !strings.HasPrefix(route.Path, "/api/") {
			continue
		}
		key := route.Method + " " + route.Path
		if !expected[key] {
			t.Fatalf("unexpected API method %s", key)
		}
		count++
		delete(expected, key)
	}
	if count != 10 || len(expected) != 0 {
		t.Fatalf("API count %d, missing %v", count, expected)
	}
	req := httptest.NewRequest("GET", "/swagger/doc.json", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if rr.Code != 200 || json.Unmarshal(rr.Body.Bytes(), &spec) != nil {
		t.Fatal("Swagger unavailable")
	}
	count = 0
	for _, operations := range spec.Paths {
		count += len(operations)
	}
	if count != 10 {
		t.Fatalf("Swagger exposes %d operations, want 10", count)
	}
	if _, ok := spec.Paths["/api/users/logout"]; ok {
		t.Fatal("extra logout method")
	}
	var op struct {
		Summary string `json:"summary"`
	}
	json.Unmarshal(spec.Paths["/api/users/authorization"]["post"], &op)
	if !strings.Contains(op.Summary, "Деавторизация") {
		t.Fatal("authorization route must revoke JWT")
	}
}
