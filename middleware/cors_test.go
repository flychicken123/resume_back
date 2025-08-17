package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestDefaultCORSConfig(t *testing.T) {
	config := DefaultCORSConfig()
	
	assert.Contains(t, config.AllowedOrigins, "*")
	assert.Contains(t, config.AllowedMethods, "GET")
	assert.Contains(t, config.AllowedMethods, "POST")
	assert.Contains(t, config.AllowedHeaders, "Content-Type")
	assert.Contains(t, config.AllowedHeaders, "Authorization")
	assert.True(t, config.AllowCredentials)
	assert.Equal(t, 86400, config.MaxAge)
}

func TestCORS_AllowAllOrigins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	config := DefaultCORSConfig()
	router := gin.New()
	router.Use(CORS(config))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})
	
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"))
}

func TestCORS_SpecificOrigins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	config := CORSConfig{
		AllowedOrigins:   []string{"http://localhost:3000", "https://example.com"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
		MaxAge:          3600,
	}
	
	router := gin.New()
	router.Use(CORS(config))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})
	
	// Allowed origin
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/test", nil)
	req1.Header.Set("Origin", "http://localhost:3000")
	router.ServeHTTP(w1, req1)
	
	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "http://localhost:3000", w1.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w1.Header().Get("Access-Control-Allow-Credentials"))
	
	// Not allowed origin
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.Header.Set("Origin", "http://notallowed.com")
	router.ServeHTTP(w2, req2)
	
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Empty(t, w2.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_PreflightRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	config := DefaultCORSConfig()
	router := gin.New()
	router.Use(CORS(config))
	router.POST("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})
	
	// OPTIONS preflight request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Headers"))
}

func TestCORS_WildcardSubdomains(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	config := CORSConfig{
		AllowedOrigins: []string{"*.example.com"},
		AllowedMethods: []string{"GET"},
		AllowedHeaders: []string{"Content-Type"},
	}
	
	router := gin.New()
	router.Use(CORS(config))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})
	
	// Subdomain should match
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/test", nil)
	req1.Header.Set("Origin", "https://app.example.com")
	router.ServeHTTP(w1, req1)
	
	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "https://app.example.com", w1.Header().Get("Access-Control-Allow-Origin"))
	
	// Another subdomain should match
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.Header.Set("Origin", "https://api.example.com")
	router.ServeHTTP(w2, req2)
	
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, "https://api.example.com", w2.Header().Get("Access-Control-Allow-Origin"))
	
	// Different domain should not match
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/test", nil)
	req3.Header.Set("Origin", "https://different.com")
	router.ServeHTTP(w3, req3)
	
	assert.Equal(t, http.StatusOK, w3.Code)
	assert.Empty(t, w3.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_ProductionEnvironment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	config := DefaultCORSConfig()
	router := gin.New()
	router.Use(CORS(config))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})
	
	// Request with X-Forwarded headers (production)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	req.Header.Set("Origin", "http://example.com")
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	// In production, CORS headers should not be set (nginx handles it)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_ExposedHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	config := CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET"},
		AllowedHeaders: []string{"Content-Type"},
		ExposedHeaders: []string{"X-Total-Count", "X-Page"},
	}
	
	router := gin.New()
	router.Use(CORS(config))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})
	
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "X-Total-Count, X-Page", w.Header().Get("Access-Control-Expose-Headers"))
}

func TestIsOriginAllowed(t *testing.T) {
	// Test exact match
	assert.True(t, isOriginAllowed("http://example.com", []string{"http://example.com"}))
	assert.False(t, isOriginAllowed("http://other.com", []string{"http://example.com"}))
	
	// Test wildcard
	assert.True(t, isOriginAllowed("http://any.com", []string{"*"}))
	
	// Test subdomain wildcard
	assert.True(t, isOriginAllowed("https://app.example.com", []string{"*.example.com"}))
	assert.True(t, isOriginAllowed("https://api.example.com", []string{"*.example.com"}))
	assert.False(t, isOriginAllowed("https://example.com", []string{"*.example.com"}))
	assert.False(t, isOriginAllowed("https://other.com", []string{"*.example.com"}))
	
	// Test empty origin
	assert.False(t, isOriginAllowed("", []string{"http://example.com"}))
}

func TestIsProductionEnvironment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	// Test with X-Forwarded-For
	c1 := &gin.Context{}
	c1.Request = httptest.NewRequest("GET", "/", nil)
	c1.Request.Header.Set("X-Forwarded-For", "192.168.1.1")
	assert.True(t, isProductionEnvironment(c1))
	
	// Test with X-Forwarded-Proto
	c2 := &gin.Context{}
	c2.Request = httptest.NewRequest("GET", "/", nil)
	c2.Request.Header.Set("X-Forwarded-Proto", "https")
	assert.True(t, isProductionEnvironment(c2))
	
	// Test without forwarded headers
	c3 := &gin.Context{}
	c3.Request = httptest.NewRequest("GET", "/", nil)
	assert.False(t, isProductionEnvironment(c3))
}