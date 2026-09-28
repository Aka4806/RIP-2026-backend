package media

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Resolver struct {
	BaseURL string
	Client  *http.Client
}

func NewResolver(baseURL string) *Resolver {
	return &Resolver{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client:  &http.Client{Timeout: time.Second},
	}
}

func (r *Resolver) Resolve(value *string, kind string) string {
	fallback := "/static/media/default.png"
	if kind == "video" {
		fallback = "/static/media/default.mp4"
	}
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	raw := strings.TrimSpace(*value)
	address, err := url.Parse(raw)
	if err != nil {
		return fallback
	}
	if address.Scheme == "" {
		address, err = url.Parse(r.BaseURL + "/" + url.PathEscape(raw))
		if err != nil {
			return fallback
		}
	}
	if (address.Scheme != "http" && address.Scheme != "https") || address.Host == "" || address.User != nil {
		return fallback
	}
	response, err := r.Client.Head(address.String())
	if err != nil {
		return fallback
	}
	defer response.Body.Close()
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	expectedType := kind + "/"
	if response.StatusCode < 200 || response.StatusCode >= 300 || response.ContentLength == 0 {
		return fallback
	}
	if !strings.HasPrefix(contentType, expectedType) && !strings.HasPrefix(contentType, "application/octet-stream") {
		return fallback
	}
	return address.String()
}
