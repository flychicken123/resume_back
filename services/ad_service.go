package services

import (
	"database/sql"
	"errors"
	"sync"
	"time"
)

// AdCooldownDuration is the minimum time between ad watches
const AdCooldownDuration = 30 * time.Minute

// AdService handles ad-related operations
type AdService struct {
	db *sql.DB
}

// In-memory cooldown tracker (resets on server restart)
var adCooldowns = make(map[int]time.Time)
var cooldownMutex sync.RWMutex


// NewAdService creates a new AdService instance
func NewAdService(db *sql.DB) *AdService {
	return &AdService{db: db}
}

// CanWatchAd checks if a user can watch an ad based on cooldown
// Returns (canWatch, remainingCooldownSeconds)
func (s *AdService) CanWatchAd(userID int) (bool, int) {
	cooldownMutex.RLock()
	lastWatch, exists := adCooldowns[userID]
	cooldownMutex.RUnlock()

	if !exists {
		return true, 0
	}

	elapsed := time.Since(lastWatch)

	if elapsed >= AdCooldownDuration {
		return true, 0
	}

	remainingSeconds := int((AdCooldownDuration - elapsed).Seconds())
	return false, remainingSeconds
}

// GrantResumeCredit decrements the user's resume usage count by 1
func (s *AdService) GrantResumeCredit(userID int) error {
	// Check cooldown first
	canWatch, _ := s.CanWatchAd(userID)
	if !canWatch {
		return errors.New("cooldown active - please wait before watching another ad")
	}

	// Decrement resume_usage count (but not below 0)
	result, err := s.db.Exec(`
		UPDATE resume_usage
		SET resume_count = GREATEST(resume_count - 1, 0),
			updated_at = NOW()
		WHERE user_id = $1
		AND period_start <= NOW()
		AND period_end > NOW()
	`, userID)

	if err != nil {
		return err
	}

	// Check if any rows were updated
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	// If no rows were affected, user might not have a usage record yet
	// This is fine - they haven't used any resumes this period
	if rowsAffected == 0 {
		// Create a new usage record with -1 count (giving them a bonus)
		// First get the user's plan to determine the period
		err = s.createBonusUsageRecord(userID)
		if err != nil {
			return err
		}
	}

	// Update cooldown tracker
	cooldownMutex.Lock()
	adCooldowns[userID] = time.Now()
	cooldownMutex.Unlock()

	return nil
}

// createBonusUsageRecord creates a usage record with negative count for users without existing records
func (s *AdService) createBonusUsageRecord(userID int) error {
	// Get user's plan period type
	var resumePeriod string
	err := s.db.QueryRow(`
		SELECT COALESCE(sp.resume_period, 'weekly')
		FROM users u
		LEFT JOIN user_subscriptions us ON u.id = us.user_id
		LEFT JOIN subscription_plans sp ON us.plan_id = sp.id
		WHERE u.id = $1
	`, userID).Scan(&resumePeriod)

	if err != nil {
		// Default to weekly if we can't determine
		resumePeriod = "weekly"
	}

	// Calculate period boundaries
	var periodStart, periodEnd time.Time
	now := time.Now()

	switch resumePeriod {
	case "daily":
		periodStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		periodEnd = periodStart.AddDate(0, 0, 1)
	case "monthly":
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		periodEnd = periodStart.AddDate(0, 1, 0)
	default: // weekly
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		periodStart = time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
		periodEnd = periodStart.AddDate(0, 0, 7)
	}

	// Insert or update the usage record with -1 (bonus credit)
	_, err = s.db.Exec(`
		INSERT INTO resume_usage (user_id, resume_count, period_start, period_end, created_at, updated_at)
		VALUES ($1, -1, $2, $3, NOW(), NOW())
		ON CONFLICT (user_id, period_start, period_end)
		DO UPDATE SET resume_count = resume_usage.resume_count - 1, updated_at = NOW()
	`, userID, periodStart, periodEnd)

	return err
}

// GetAdStatus returns the current ad status for a user
type AdStatus struct {
	CanWatchAd              bool `json:"can_watch_ad"`
	CooldownRemainingSeconds int  `json:"cooldown_remaining_seconds"`
}

func (s *AdService) GetAdStatus(userID int) *AdStatus {
	canWatch, remaining := s.CanWatchAd(userID)
	return &AdStatus{
		CanWatchAd:              canWatch,
		CooldownRemainingSeconds: remaining,
	}
}
