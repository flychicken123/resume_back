package services

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"
)

// CareerField represents a candidate's career category
type CareerField string

const (
	CareerFieldUnknown            CareerField = "UNKNOWN"
	CareerFieldSoftwareEngineering CareerField = "SOFTWARE_ENGINEERING"
	CareerFieldDataScience        CareerField = "DATA_SCIENCE"
	CareerFieldProductManagement  CareerField = "PRODUCT_MANAGEMENT"
	CareerFieldDesign             CareerField = "DESIGN"
	CareerFieldSales              CareerField = "SALES"
	CareerFieldMarketing          CareerField = "MARKETING"
	CareerFieldFinance            CareerField = "FINANCE"
	CareerFieldOperations         CareerField = "OPERATIONS"
	CareerFieldHRRecruiting       CareerField = "HR_RECRUITING"
	CareerFieldCustomerSuccess    CareerField = "CUSTOMER_SUCCESS"
	CareerFieldOther              CareerField = "OTHER"
)

const (
	careerFieldCacheTTL   = 24 * time.Hour // Cache candidate career field for 24 hours
	jobRelevanceCacheTTL  = 7 * 24 * time.Hour // Cache job relevance for 7 days
	cacheCleanupInterval  = 1 * time.Hour
	// Cache version - increment this when prompt logic changes to invalidate old cache entries
	cacheVersion          = "v2" // v2: less aggressive filtering prompt
)

// jobFilterCacheEntry holds a cached value with expiration
type jobFilterCacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

// JobFilterCache provides two-level caching for job filtering
type JobFilterCache struct {
	// Level 1: Candidate career field cache
	// Key: hash(position + skills) → CareerField
	careerFieldCache sync.Map

	// Level 2: Job relevance cache
	// Key: "career_field:job_id" → bool (relevant or not)
	jobRelevanceCache sync.Map

	// Stats
	careerFieldHits   int64
	careerFieldMisses int64
	jobRelevanceHits  int64
	jobRelevanceMisses int64
	statsMu           sync.Mutex
}

// Global cache instance
var (
	jobFilterCache     *JobFilterCache
	jobFilterCacheOnce sync.Once
)

// GetJobFilterCache returns the singleton cache instance
func GetJobFilterCache() *JobFilterCache {
	jobFilterCacheOnce.Do(func() {
		jobFilterCache = &JobFilterCache{}
		// Start background cleanup goroutine
		go jobFilterCache.periodicCleanup()
	})
	return jobFilterCache
}

// generateCandidateHash creates a hash from candidate's position and skills
func generateCandidateHash(position string, skills []string) string {
	// Normalize inputs
	pos := strings.ToLower(strings.TrimSpace(position))

	// Sort skills for consistent hashing
	normalizedSkills := make([]string, 0, len(skills))
	for _, s := range skills {
		normalized := strings.ToLower(strings.TrimSpace(s))
		if normalized != "" {
			normalizedSkills = append(normalizedSkills, normalized)
		}
	}
	sort.Strings(normalizedSkills)

	// Create hash input
	hashInput := pos + "|" + strings.Join(normalizedSkills, ",")

	hash := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(hash[:16]) // Use first 16 bytes for shorter key
}


// GetCareerField retrieves cached career field for a candidate
func (c *JobFilterCache) GetCareerField(position string, skills []string) (CareerField, bool) {
	hash := generateCandidateHash(position, skills)

	if entry, ok := c.careerFieldCache.Load(hash); ok {
		cached := entry.(jobFilterCacheEntry)
		if time.Now().Before(cached.expiresAt) {
			c.statsMu.Lock()
			c.careerFieldHits++
			c.statsMu.Unlock()
			return cached.value.(CareerField), true
		}
		// Expired, delete it
		c.careerFieldCache.Delete(hash)
	}

	c.statsMu.Lock()
	c.careerFieldMisses++
	c.statsMu.Unlock()
	return CareerFieldUnknown, false
}

// SetCareerField caches the career field for a candidate
func (c *JobFilterCache) SetCareerField(position string, skills []string, field CareerField) {
	hash := generateCandidateHash(position, skills)
	c.careerFieldCache.Store(hash, jobFilterCacheEntry{
		value:     field,
		expiresAt: time.Now().Add(careerFieldCacheTTL),
	})
}

// GetJobRelevance retrieves cached job relevance for a career field
func (c *JobFilterCache) GetJobRelevance(careerField CareerField, jobID int64) (bool, bool) {
	key := cacheVersion + ":" + string(careerField) + ":" + formatJobID(jobID)

	if entry, ok := c.jobRelevanceCache.Load(key); ok {
		cached := entry.(jobFilterCacheEntry)
		if time.Now().Before(cached.expiresAt) {
			c.statsMu.Lock()
			c.jobRelevanceHits++
			c.statsMu.Unlock()
			return cached.value.(bool), true
		}
		// Expired, delete it
		c.jobRelevanceCache.Delete(key)
	}

	c.statsMu.Lock()
	c.jobRelevanceMisses++
	c.statsMu.Unlock()
	return false, false
}

// SetJobRelevance caches job relevance for a career field
func (c *JobFilterCache) SetJobRelevance(careerField CareerField, jobID int64, relevant bool) {
	key := cacheVersion + ":" + string(careerField) + ":" + formatJobID(jobID)
	c.jobRelevanceCache.Store(key, jobFilterCacheEntry{
		value:     relevant,
		expiresAt: time.Now().Add(jobRelevanceCacheTTL),
	})
}

// SetBulkJobRelevance caches multiple job relevance results at once
func (c *JobFilterCache) SetBulkJobRelevance(careerField CareerField, relevantJobIDs []int64, irrelevantJobIDs []int64) {
	for _, jobID := range relevantJobIDs {
		c.SetJobRelevance(careerField, jobID, true)
	}
	for _, jobID := range irrelevantJobIDs {
		c.SetJobRelevance(careerField, jobID, false)
	}
}

// GetStats returns cache statistics
func (c *JobFilterCache) GetStats() map[string]int64 {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	return map[string]int64{
		"careerFieldHits":    c.careerFieldHits,
		"careerFieldMisses":  c.careerFieldMisses,
		"jobRelevanceHits":   c.jobRelevanceHits,
		"jobRelevanceMisses": c.jobRelevanceMisses,
	}
}

// periodicCleanup removes expired entries periodically
func (c *JobFilterCache) periodicCleanup() {
	ticker := time.NewTicker(cacheCleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		// Cleanup career field cache
		c.careerFieldCache.Range(func(key, value interface{}) bool {
			entry := value.(jobFilterCacheEntry)
			if now.After(entry.expiresAt) {
				c.careerFieldCache.Delete(key)
			}
			return true
		})

		// Cleanup job relevance cache
		c.jobRelevanceCache.Range(func(key, value interface{}) bool {
			entry := value.(jobFilterCacheEntry)
			if now.After(entry.expiresAt) {
				c.jobRelevanceCache.Delete(key)
			}
			return true
		})
	}
}

// formatJobID converts job ID to string for cache key
func formatJobID(jobID int64) string {
	// Simple int to string conversion
	if jobID == 0 {
		return "0"
	}

	negative := jobID < 0
	if negative {
		jobID = -jobID
	}

	var buf [20]byte
	i := len(buf)
	for jobID > 0 {
		i--
		buf[i] = byte(jobID%10) + '0'
		jobID /= 10
	}

	if negative {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}

// ClearCache clears all cached entries (useful for testing)
func (c *JobFilterCache) ClearCache() {
	c.careerFieldCache = sync.Map{}
	c.jobRelevanceCache = sync.Map{}
	c.statsMu.Lock()
	c.careerFieldHits = 0
	c.careerFieldMisses = 0
	c.jobRelevanceHits = 0
	c.jobRelevanceMisses = 0
	c.statsMu.Unlock()
}
