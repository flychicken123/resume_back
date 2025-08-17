package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestNewResponseCache(t *testing.T) {
	cache := NewResponseCache(5 * time.Minute)
	
	assert.NotNil(t, cache)
	assert.NotNil(t, cache.cache)
	assert.Equal(t, 5*time.Minute, cache.ttl)
}

func TestResponseCache_CacheGETRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	cache := NewResponseCache(1 * time.Minute)
	router := gin.New()
	
	callCount := 0
	router.Use(cache.Cache())
	router.GET("/test", func(c *gin.Context) {
		callCount++
		c.JSON(200, gin.H{"count": callCount})
	})
	
	// First request - should hit handler
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w1, req1)
	
	var resp1 map[string]int
	err := json.Unmarshal(w1.Body.Bytes(), &resp1)
	assert.NoError(t, err)
	assert.Equal(t, 1, resp1["count"])
	
	// Second request - should be cached
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w2, req2)
	
	var resp2 map[string]int
	err = json.Unmarshal(w2.Body.Bytes(), &resp2)
	assert.NoError(t, err)
	assert.Equal(t, 1, resp2["count"]) // Should still be 1 (cached)
}

func TestResponseCache_DifferentKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	cache := NewResponseCache(1 * time.Minute)
	router := gin.New()
	router.Use(cache.Cache())
	
	router.GET("/test", func(c *gin.Context) {
		query := c.Query("q")
		c.JSON(200, gin.H{"query": query})
	})
	
	// Request with query param "a"
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/test?q=a", nil)
	router.ServeHTTP(w1, req1)
	
	var resp1 map[string]string
	err := json.Unmarshal(w1.Body.Bytes(), &resp1)
	assert.NoError(t, err)
	assert.Equal(t, "a", resp1["query"])
	
	// Request with query param "b" - different cache key
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test?q=b", nil)
	router.ServeHTTP(w2, req2)
	
	var resp2 map[string]string
	err = json.Unmarshal(w2.Body.Bytes(), &resp2)
	assert.NoError(t, err)
	assert.Equal(t, "b", resp2["query"])
}

func TestResponseCache_Expiration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	// Very short TTL for testing
	cache := NewResponseCache(100 * time.Millisecond)
	router := gin.New()
	
	callCount := 0
	router.Use(cache.Cache())
	router.GET("/test", func(c *gin.Context) {
		callCount++
		c.JSON(200, gin.H{"count": callCount})
	})
	
	// First request
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w1, req1)
	
	var resp1 map[string]int
	err := json.Unmarshal(w1.Body.Bytes(), &resp1)
	assert.NoError(t, err)
	assert.Equal(t, 1, resp1["count"])
	
	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)
	
	// Second request - should hit handler again
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w2, req2)
	
	var resp2 map[string]int
	err = json.Unmarshal(w2.Body.Bytes(), &resp2)
	assert.NoError(t, err)
	assert.Equal(t, 2, resp2["count"]) // Should be 2 (not cached)
}

func TestResponseCache_OnlyCache200Status(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	cache := NewResponseCache(1 * time.Minute)
	router := gin.New()
	
	callCount := 0
	router.Use(cache.Cache())
	router.GET("/test/:status", func(c *gin.Context) {
		callCount++
		status := c.Param("status")
		if status == "404" {
			c.JSON(404, gin.H{"error": "not found", "count": callCount})
		} else {
			c.JSON(200, gin.H{"message": "ok", "count": callCount})
		}
	})
	
	// Request that returns 404
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/test/404", nil)
	router.ServeHTTP(w1, req1)
	assert.Equal(t, 404, w1.Code)
	
	// Second request to same endpoint - should NOT be cached
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test/404", nil)
	router.ServeHTTP(w2, req2)
	
	var resp2 map[string]interface{}
	err := json.Unmarshal(w2.Body.Bytes(), &resp2)
	assert.NoError(t, err)
	assert.Equal(t, float64(2), resp2["count"]) // Should be 2 (not cached)
}

func TestResponseCache_AIEndpoint(t *testing.T) {
	// Test that AI endpoints are properly identified
	assert.True(t, isAIEndpoint("/api/experience/optimize"))
	assert.True(t, isAIEndpoint("/api/ai/education"))
	assert.True(t, isAIEndpoint("/api/ai/summary"))
	assert.False(t, isAIEndpoint("/api/auth/login"))
	assert.False(t, isAIEndpoint("/api/user/profile"))
}

func TestResponseCache_POSTRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	cache := NewResponseCache(1 * time.Minute)
	router := gin.New()
	
	callCount := 0
	router.Use(cache.Cache())
	router.POST("/api/ai/summary", func(c *gin.Context) {
		callCount++
		var body map[string]string
		_ = c.BindJSON(&body)
		c.JSON(200, gin.H{"processed": body["text"], "count": callCount})
	})
	
	// First POST request
	body1 := map[string]string{"text": "test1"}
	jsonBody1, _ := json.Marshal(body1)
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("POST", "/api/ai/summary", bytes.NewBuffer(jsonBody1))
	req1.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w1, req1)
	
	var resp1 map[string]interface{}
	json.Unmarshal(w1.Body.Bytes(), &resp1)
	assert.Equal(t, float64(1), resp1["count"])
	
	// Same POST request - should be cached for AI endpoint
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/ai/summary", bytes.NewBuffer(jsonBody1))
	req2.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w2, req2)
	
	var resp2 map[string]interface{}
	err := json.Unmarshal(w2.Body.Bytes(), &resp2)
	assert.NoError(t, err)
	assert.Equal(t, float64(1), resp2["count"]) // Should still be 1 (cached)
}

func TestCreateCaches(t *testing.T) {
	caches := CreateCaches()
	
	assert.NotNil(t, caches["ai"])
	assert.NotNil(t, caches["general"])
	
	// Check TTLs
	assert.Equal(t, 15*time.Minute, caches["ai"].ttl)
	assert.Equal(t, 5*time.Minute, caches["general"].ttl)
}