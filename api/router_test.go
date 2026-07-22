package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeadersDisableAPICaching(t *testing.T) {
	router := gin.New()
	router.Use(SecurityHeaders())
	router.GET("/api/example", func(c *gin.Context) { c.Status(http.StatusOK) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/example", nil)
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Cache-Control"); got != "no-store, max-age=0" {
		t.Fatalf("Cache-Control = %q, want no-store, max-age=0", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestSecurityHeadersKeepNonAPIAssetsCachePolicyUntouched(t *testing.T) {
	router := gin.New()
	router.Use(SecurityHeaders())
	router.GET("/assets/example.js", func(c *gin.Context) { c.Status(http.StatusOK) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/assets/example.js", nil)
	router.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("asset Cache-Control = %q, want empty", got)
	}
}
