package middleware

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
	"github.com/gin-gonic/gin"
)

// CacheEntry represents a cached response
type CacheEntry struct {
	Data      interface{}
	ExpiresAt time.Time
}

// ResponseCache manages cached responses
type ResponseCache struct {
	cache map[string]*CacheEntry
	mu    sync.RWMutex
	ttl   time.Duration
}

// NewResponseCache creates a new response cache
func NewResponseCache(ttl time.Duration) *ResponseCache {
	rc := &ResponseCache{
		cache: make(map[string]*CacheEntry),
		ttl:   ttl,
	}
	
	// Clean up expired entries every 5 minutes
	go rc.cleanup()
	
	return rc
}

// Cache middleware for caching responses
func (rc *ResponseCache) Cache() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only cache GET requests and successful POST AI responses
		if c.Request.Method != "GET" && !isAIEndpoint(c.Request.URL.Path) {
			c.Next()
			return
		}
		
		// Generate cache key
		key := rc.generateKey(c)
		
		// Check if cached response exists
		rc.mu.RLock()
		entry, exists := rc.cache[key]
		rc.mu.RUnlock()
		
		if exists && time.Now().Before(entry.ExpiresAt) {
			// Return cached response
			c.JSON(200, entry.Data)
			c.Abort()
			return
		}
		
		// Create response writer wrapper to capture response
		writer := &responseWriter{
			ResponseWriter: c.Writer,
			body:          []byte{},
		}
		c.Writer = writer
		
		// Process request
		c.Next()
		
		// Cache successful responses
		if c.Writer.Status() == 200 && len(writer.body) > 0 {
			var data interface{}
			if err := json.Unmarshal(writer.body, &data); err == nil {
				rc.mu.Lock()
				rc.cache[key] = &CacheEntry{
					Data:      data,
					ExpiresAt: time.Now().Add(rc.ttl),
				}
				rc.mu.Unlock()
			}
		}
	}
}

// generateKey creates a cache key from request
func (rc *ResponseCache) generateKey(c *gin.Context) string {
	h := md5.New()
	h.Write([]byte(c.Request.Method))
	h.Write([]byte(c.Request.URL.Path))
	h.Write([]byte(c.Request.URL.RawQuery))
	
	// Include body for POST requests
	if c.Request.Method == "POST" {
		body, _ := c.GetRawData()
		h.Write(body)
		// Restore body for further processing
		c.Request.Body = &cachedBody{bytes: body}
	}
	
	return hex.EncodeToString(h.Sum(nil))
}

// cleanup removes expired cache entries
func (rc *ResponseCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for {
		<-ticker.C
		rc.mu.Lock()
		now := time.Now()
		for key, entry := range rc.cache {
			if now.After(entry.ExpiresAt) {
				delete(rc.cache, key)
			}
		}
		rc.mu.Unlock()
	}
}

// responseWriter wraps gin.ResponseWriter to capture response body
type responseWriter struct {
	gin.ResponseWriter
	body []byte
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return w.ResponseWriter.Write(b)
}

// cachedBody implements io.ReadCloser
type cachedBody struct {
	bytes []byte
	pos   int
}

func (cb *cachedBody) Read(p []byte) (n int, err error) {
	if cb.pos >= len(cb.bytes) {
		return 0, nil
	}
	n = copy(p, cb.bytes[cb.pos:])
	cb.pos += n
	return n, nil
}

func (cb *cachedBody) Close() error {
	return nil
}

// isAIEndpoint checks if the endpoint is an AI endpoint
func isAIEndpoint(path string) bool {
	aiPaths := []string{
		"/api/experience/optimize",
		"/api/ai/education",
		"/api/ai/summary",
		"/api/experience/improve-grammar",
		"/api/summary/improve-grammar",
	}
	
	for _, aiPath := range aiPaths {
		if path == aiPath {
			return true
		}
	}
	return false
}

// CreateCaches creates different caches for different purposes
func CreateCaches() map[string]*ResponseCache {
	return map[string]*ResponseCache{
		"ai":      NewResponseCache(15 * time.Minute), // Cache AI responses for 15 minutes
		"general": NewResponseCache(5 * time.Minute),  // Cache general responses for 5 minutes
	}
}