package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(5, 1*time.Minute)
	
	assert.NotNil(t, rl)
	assert.Equal(t, 5, rl.rate)
	assert.Equal(t, 1*time.Minute, rl.window)
	assert.NotNil(t, rl.visitors)
}

func TestRateLimiter_SingleRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	rl := NewRateLimiter(5, 1*time.Minute)
	router := gin.New()
	router.Use(rl.Limit())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})
	
	// First request should pass
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

func TestRateLimiter_MultipleRequestsWithinLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	rl := NewRateLimiter(5, 1*time.Minute)
	router := gin.New()
	router.Use(rl.Limit())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})
	
	// Make 5 requests (within limit)
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code, "Request %d should succeed", i+1)
	}
}

func TestRateLimiter_ExceedLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	rl := NewRateLimiter(3, 1*time.Minute)
	router := gin.New()
	router.Use(rl.Limit())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})
	
	// Make 3 requests (within limit)
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		router.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
	}
	
	// 4th request should be rate limited
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	router.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "Rate limit exceeded")
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	rl := NewRateLimiter(2, 1*time.Minute)
	router := gin.New()
	router.Use(rl.Limit())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})
	
	// Two requests from IP1
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
	
	// Two requests from IP2 (different IP, should work)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.2:12345"
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
	
	// Third request from IP1 should be blocked
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestRateLimiter_WindowReset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	// Use a very short window for testing
	rl := NewRateLimiter(2, 100*time.Millisecond)
	router := gin.New()
	router.Use(rl.Limit())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})
	
	// Make 2 requests (reach limit)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
	
	// 3rd request should be blocked
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	
	// Wait for window to reset
	time.Sleep(150 * time.Millisecond)
	
	// Request should work again after window reset
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCreateRateLimiters(t *testing.T) {
	limiters := CreateRateLimiters()
	
	assert.NotNil(t, limiters["ai"])
	assert.NotNil(t, limiters["auth"])
	assert.NotNil(t, limiters["general"])
	
	// Check rates
	assert.Equal(t, 10, limiters["ai"].rate)
	assert.Equal(t, 5, limiters["auth"].rate)
	assert.Equal(t, 60, limiters["general"].rate)
}