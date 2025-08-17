package middleware

import (
	"net/http"
	"sync"
	"time"
	"github.com/gin-gonic/gin"
)

// RateLimiter stores rate limit data
type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.RWMutex
	rate     int           // requests per window
	window   time.Duration // time window
}

// visitor tracks requests from a specific IP
type visitor struct {
	lastSeen time.Time
	count    int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
	}
	
	// Clean up old visitors every minute
	go rl.cleanupVisitors()
	
	return rl
}

// Limit returns a middleware that rate limits requests
func (rl *RateLimiter) Limit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		
		rl.mu.Lock()
		v, exists := rl.visitors[ip]
		if !exists {
			rl.visitors[ip] = &visitor{
				lastSeen: time.Now(),
				count:    1,
			}
			rl.mu.Unlock()
			c.Next()
			return
		}
		
		// Reset count if window has passed
		if time.Since(v.lastSeen) > rl.window {
			v.count = 1
			v.lastSeen = time.Now()
			rl.mu.Unlock()
			c.Next()
			return
		}
		
		// Check if rate limit exceeded
		if v.count >= rl.rate {
			rl.mu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded. Please try again later.",
				"retry_after": rl.window.Seconds(),
			})
			c.Abort()
			return
		}
		
		// Increment count
		v.count++
		v.lastSeen = time.Now()
		rl.mu.Unlock()
		
		c.Next()
	}
}

// cleanupVisitors removes old visitor entries
func (rl *RateLimiter) cleanupVisitors() {
	ticker := time.NewTicker(1 * time.Minute)
	for {
		<-ticker.C
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > rl.window*2 {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// CreateRateLimiters creates different rate limiters for different endpoints
func CreateRateLimiters() map[string]*RateLimiter {
	return map[string]*RateLimiter{
		"ai":      NewRateLimiter(10, 1*time.Minute),  // 10 requests per minute for AI
		"auth":    NewRateLimiter(5, 1*time.Minute),   // 5 requests per minute for auth
		"general": NewRateLimiter(60, 1*time.Minute),  // 60 requests per minute general
	}
}