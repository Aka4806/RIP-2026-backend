package media

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolve(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/image with spaces.png":
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Content-Length", "10")
		case "/video.MOV":
			w.Header().Set("Content-Type", "video/quicktime")
			w.Header().Set("Content-Length", "10")
		case "/broken":
			w.WriteHeader(http.StatusNotFound)
		case "/html":
			w.Header().Set("Content-Type", "text/html")
		case "/slow":
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	resolver := NewResolver(server.URL)
	tests := []struct{ value, kind, want string }{
		{"", "image", "/static/media/default.png"},
		{"   ", "video", "/static/media/default.mp4"},
		{server.URL + "/image with spaces.png", "image", server.URL + "/image%20with%20spaces.png"},
		{"image with spaces.png", "image", server.URL + "/image%20with%20spaces.png"},
		{server.URL + "/video.MOV", "video", server.URL + "/video.MOV"},
		{server.URL + "/broken", "video", "/static/media/default.mp4"},
		{server.URL + "/html", "image", "/static/media/default.png"},
		{"javascript:alert(1)", "image", "/static/media/default.png"},
	}
	for _, test := range tests {
		if got := resolver.Resolve(&test.value, test.kind); got != test.want {
			t.Errorf("%s: got %s, want %s", test.value, got, test.want)
		}
	}
	if got := resolver.Resolve(nil, "image"); !strings.HasSuffix(got, "default.png") {
		t.Fatal(got)
	}
	resolver.Client.Timeout = 20 * time.Millisecond
	slow := server.URL + "/slow"
	if got := resolver.Resolve(&slow, "video"); got != "/static/media/default.mp4" {
		t.Fatal(got)
	}
}
