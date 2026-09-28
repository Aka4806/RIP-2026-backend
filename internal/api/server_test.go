package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"awesomeProject/internal/app/models"
	"awesomeProject/internal/database"
)

func TestCardLifecycle(t *testing.T) {
	if os.Getenv("TEST_DATABASE") != "1" {
		t.Skip("set TEST_DATABASE=1 to test PostgreSQL")
	}
	t.Chdir("../..")
	db, err := database.Connect()
	if err != nil {
		t.Fatal(err)
	}
	connection, _ := db.DB()
	defer connection.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	user := models.User{Username: fmt.Sprintf("test_%d", time.Now().UnixNano())}
	if err := tx.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	router := NewRouter(tx, user.ID)
	call := func(method, path string, form url.Values, status int) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		if method == http.MethodPost {
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != status {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, response.Code, status, response.Body.String())
		}
		return response
	}
	var before, after int64
	tx.Model(&models.IndexConfiguration{}).Count(&before)
	empty := call("GET", "/add", nil, 200).Body.String()
	tx.Model(&models.IndexConfiguration{}).Count(&after)
	if before != after || !strings.Contains(empty, "Далее") || strings.Contains(empty, `name="table_size"`) {
		t.Fatal("empty GET must not create draft")
	}
	if strings.Contains(empty, `name="image`) || strings.Contains(empty, `name="video`) || !strings.Contains(empty, `type="file"`) {
		t.Fatal("media selectors must not transmit files")
	}
	call("POST", "/indexes", url.Values{"name": {" "}}, 400)
	call("POST", "/indexes", url.Values{"name": {strings.Repeat("Я", 151)}}, 400)
	create := url.Values{"name": {"Тестовый индекс"}, "image_url": {"https://ignored/image.png"}, "video_url": {"https://ignored/video.mp4"}}
	if response := call("POST", "/indexes", create, 303); response.Header().Get("Location") != "/add" {
		t.Fatal("create redirect")
	}
	var item models.IndexConfiguration
	if err := tx.Where("creator_id = ? AND status = ?", user.ID, models.Draft).First(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.ImageURL != nil || item.VideoURL != nil || item.FormedAt != nil || item.CreatedAt.IsZero() {
		t.Fatal("invalid initial draft state")
	}
	call("POST", "/indexes", create, 303)
	var drafts int64
	tx.Model(&models.IndexConfiguration{}).Where("creator_id = ? AND status = ?", user.ID, models.Draft).Count(&drafts)
	if drafts != 1 {
		t.Fatalf("duplicate draft: %d", drafts)
	}
	id := strconv.Itoa(item.ID)
	draft := call("GET", "/add", nil, 200).Body.String()
	if !strings.Contains(draft, "Опубликовать") || !strings.Contains(draft, "/static/media/default.mp4") {
		t.Fatal("draft form")
	}
	call("GET", "/feed/"+id, nil, 404)
	publishURL := "/indexes/" + id + "/publish"
	valid := url.Values{"description": {"Описание индекса"}, "table_size": {"1000000"}, "selectivity": {"0.01"}}
	for _, invalid := range []string{"NaN", "Inf", "-0.1", "1.1", "abc"} {
		form := url.Values{"description": {"Сохранить введённый текст"}, "table_size": {"1000000"}, "selectivity": {invalid}}
		body := call("POST", publishURL, form, 400).Body.String()
		if !strings.Contains(body, "Сохранить введённый текст") {
			t.Fatal("lost input after validation")
		}
	}
	call("POST", publishURL, url.Values{"description": {"text"}, "table_size": {"2147483648"}, "selectivity": {"0.01"}}, 400)
	call("POST", publishURL, url.Values{"description": {""}, "table_size": {"1000000"}, "selectivity": {"0.01"}}, 400)
	foreignRouter := NewRouter(tx, user.ID+1)
	response := httptest.NewRecorder()
	request := httptest.NewRequest("POST", publishURL, strings.NewReader(valid.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	foreignRouter.ServeHTTP(response, request)
	if response.Code != 404 {
		t.Fatal("another user can publish draft")
	}
	response = call("POST", publishURL, valid, 303)
	if response.Header().Get("Location") != "/feed/"+id {
		t.Fatal("publish redirect")
	}
	if err := tx.First(&item, item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if item.Status != models.Published || item.FormedAt == nil || item.TableSize != 1000000 || item.ShortDescription == nil || *item.ShortDescription != "Описание индекса" {
		t.Fatal("publication not saved")
	}
	call("POST", publishURL, valid, 404)
	feed := call("GET", "/feed/"+id, nil, 200).Body.String()
	if !strings.Contains(feed, "/static/media/default.mp4") {
		t.Fatal("feed fallback")
	}
	grid := call("GET", "/indexes?size_step=1", nil, 200).Body.String()
	if !strings.Contains(grid, "/feed/"+id) || !strings.Contains(grid, "/indexes/"+id+"/delete") {
		t.Fatal("grid publication and delete button")
	}
	if strings.Contains(call("GET", "/indexes?size_step=0", nil, 200).Body.String(), "/feed/"+id+`"`) {
		t.Fatal("filter ignored")
	}
	call("GET", "/indexes?size_step=no", nil, 400)
	like := models.IndexConfigurationLike{UserID: user.ID, IndexConfigurationID: item.ID}
	if err := tx.Create(&like).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(call("GET", "/feed/"+id, nil, 200).Body.String(), "<strong>1</strong>") {
		t.Fatal("likes not from DB")
	}
	if err := tx.Model(&item).Update("selectivity", 0.123).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(call("GET", "/feed/"+id, nil, 200).Body.String(), "0.123") {
		t.Fatal("DB updates not reflected")
	}
	deleted := call("POST", "/indexes/"+id+"/delete", url.Values{"size_step": {"1"}}, 303)
	if deleted.Header().Get("Location") != "/indexes?size_step=1" {
		t.Fatal("filter not preserved")
	}
	if err := tx.First(&item, item.ID).Error; err != nil {
		t.Fatal("row physically deleted", err)
	}
	if item.Status != models.Deleted {
		t.Fatal("logical deletion not saved")
	}
	if err := tx.First(&like, like.ID).Error; err != nil {
		t.Fatal("likes lost", err)
	}
	call("GET", "/feed/"+id, nil, 404)
	call("GET", "/feed/"+id+"?next=true", nil, 404)
	call("POST", "/indexes/"+id+"/delete", url.Values{}, 404)
	call("POST", publishURL, valid, 404)
	call("POST", "/indexes/bad/delete", url.Values{}, 400)
	if strings.Contains(call("GET", "/indexes", nil, 200).Body.String(), "/feed/"+id+`"`) {
		t.Fatal("deleted card visible")
	}
	call("POST", "/indexes", create, 303)
	call("POST", "/indexes", create, 303)
	tx.Model(&models.IndexConfiguration{}).Where("creator_id = ? AND status = ?", user.ID, models.Draft).Count(&drafts)
	if drafts != 1 {
		t.Fatal("next draft creation")
	}
	request = httptest.NewRequest("POST", "/indexes", strings.NewReader(create.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "http://foreign.example")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("cross-site POST accepted")
	}
	request = httptest.NewRequest("POST", "/indexes", strings.NewReader("file"))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 415 {
		t.Fatal("file upload accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request = httptest.NewRequest("GET", "/indexes", nil).WithContext(ctx)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 500 {
		t.Fatal("database failure must not look like not-found")
	}
	getCount, postCount := 0, 0
	for _, route := range router.Routes() {
		if strings.HasPrefix(route.Path, "/static/") {
			continue
		}
		if route.Method == "GET" {
			getCount++
		}
		if route.Method == "POST" {
			postCount++
		}
	}
	if getCount != 3 || postCount != 3 {
		t.Fatalf("routes: %d GET, %d POST", getCount, postCount)
	}
}
