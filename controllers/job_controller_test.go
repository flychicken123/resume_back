package controllers

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDetectCountryFromHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/jobs/match", nil)
	ctx.Request.Header.Set("CF-IPCountry", "US")

	if code := detectCountryFromRequest(ctx); code != "us" {
		t.Fatalf("expected header-derived country to be us, got %s", code)
	}
}

func TestDetectCountryFromAcceptLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/jobs/match", nil)
	ctx.Request.Header.Set("Accept-Language", "en-GB,en;q=0.9")

	if code := detectCountryFromRequest(ctx); code != "uk" {
		t.Fatalf("expected accept-language derived country to be uk, got %s", code)
	}
}
