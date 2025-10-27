package controllers

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type rowScanner interface {
	Scan(dest ...any) error
}

type AdminController struct {
	db *sql.DB
}

func NewAdminController(db *sql.DB) *AdminController {
	return &AdminController{db: db}
}

const adminMembershipSelect = `
SELECT
    u.id,
    u.email,
    u.name,
    u.is_admin,
    COALESCE(u.subscription_plan_id, sp.id) AS plan_id,
    sp.name,
    sp.display_name,
    u.subscription_status,
    COALESCE(us.status, u.subscription_status) AS billing_status,
    sp.resume_limit,
    sp.resume_period,
    u.trial_end_date,
    u.stripe_customer_id,
    us.stripe_subscription_id,
    us.current_period_start,
    us.current_period_end,
    COALESCE(us.cancel_at_period_end, false) AS cancel_at_period_end,
    u.marketing_opt_in,
    u.signup_plan_preference,
    u.created_at,
    u.updated_at
FROM users u
LEFT JOIN subscription_plans sp ON sp.id = COALESCE(u.subscription_plan_id, 1)
LEFT JOIN user_subscriptions us ON us.user_id = u.id
`

func (ac *AdminController) ListUsers(c *gin.Context) {
	rows, err := ac.db.Query(adminMembershipSelect + " ORDER BY u.created_at DESC")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch users"})
		return
	}
	defer rows.Close()

	var users []gin.H
	for rows.Next() {
		record, err := ac.scanMembership(rows)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse user record"})
			return
		}
		users = append(users, record)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read user records"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

func (ac *AdminController) UpdateUserMembership(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req struct {
		PlanName          string  `json:"plan_name"`
		PlanID            *int    `json:"plan_id"`
		Status            string  `json:"status"`
		CancelAtPeriodEnd *bool   `json:"cancel_at_period_end"`
		TrialEndDate      *string `json:"trial_end_date"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	if req.PlanName == "" && req.PlanID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plan_name or plan_id required"})
		return
	}

	var plan struct {
		ID           int
		Name         string
		ResumePeriod string
	}

	if req.PlanID != nil {
		err = ac.db.QueryRow(`SELECT id, name, resume_period FROM subscription_plans WHERE id = $1`, *req.PlanID).Scan(&plan.ID, &plan.Name, &plan.ResumePeriod)
	} else {
		err = ac.db.QueryRow(`SELECT id, name, resume_period FROM subscription_plans WHERE LOWER(name) = LOWER($1)`, req.PlanName).Scan(&plan.ID, &plan.Name, &plan.ResumePeriod)
	}

	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "plan not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load plan"})
		return
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		if strings.EqualFold(plan.Name, "free") {
			status = "free"
		} else {
			status = "active"
		}
	}

	cancelAtPeriodEnd := false
	if req.CancelAtPeriodEnd != nil {
		cancelAtPeriodEnd = *req.CancelAtPeriodEnd
	}

	var trialEndValue interface{}
	if req.TrialEndDate != nil {
		trialRaw := strings.TrimSpace(*req.TrialEndDate)
		if trialRaw != "" {
			parsed, parseErr := time.Parse(time.RFC3339, trialRaw)
			if parseErr != nil {
				parsed, parseErr = time.Parse("2006-01-02", trialRaw)
				if parseErr != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid trial_end_date format"})
					return
				}
			}
			trialEndValue = parsed.UTC()
		}
	}

	periodStart := time.Now().UTC()
	periodEnd := periodStart.AddDate(0, 1, 0)

	switch strings.ToLower(plan.ResumePeriod) {
	case "weekly":
		periodEnd = periodStart.AddDate(0, 0, 7)
	case "daily":
		periodEnd = periodStart.AddDate(0, 0, 1)
	}

	tx, err := ac.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start transaction"})
		return
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			fmt.Printf("admin membership rollback failed: %v\n", rbErr)
		}
	}()

	result, err := tx.Exec(`
        UPDATE users
        SET subscription_plan_id = $1,
            subscription_status = $2,
            trial_end_date = $3,
            updated_at = NOW()
        WHERE id = $4
    `, plan.ID, status, trialEndValue, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	_, err = tx.Exec(`
        INSERT INTO user_subscriptions (
            user_id, plan_id, status, current_period_start, current_period_end, cancel_at_period_end
        ) VALUES ($1, $2, $3, $4, $5, $6)
        ON CONFLICT (user_id) DO UPDATE SET
            plan_id = EXCLUDED.plan_id,
            status = EXCLUDED.status,
            current_period_start = EXCLUDED.current_period_start,
            current_period_end = EXCLUDED.current_period_end,
            cancel_at_period_end = EXCLUDED.cancel_at_period_end,
            updated_at = NOW()
    `, userID, plan.ID, status, periodStart, periodEnd, cancelAtPeriodEnd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist membership"})
		return
	}

	if strings.EqualFold(plan.Name, "free") {
		if _, err := tx.Exec(`UPDATE user_subscriptions SET stripe_subscription_id = NULL, stripe_customer_id = NULL WHERE user_id = $1`, userID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear billing data"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finalize membership update"})
		return
	}

	record, err := ac.fetchUserMembership(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load updated membership"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "membership updated",
		"user":    record,
	})
}

func (ac *AdminController) fetchUserMembership(userID int) (gin.H, error) {
	row := ac.db.QueryRow(adminMembershipSelect+" WHERE u.id = $1", userID)
	return ac.scanMembership(row)
}

func (ac *AdminController) ExportUserEmails(c *gin.Context) {
	includeAll := strings.EqualFold(c.Query("all"), "true")
	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "json")))

	query := `SELECT email, COALESCE(name, ''), marketing_opt_in, COALESCE(signup_plan_preference, '') FROM users`
	if !includeAll {
		query += " WHERE marketing_opt_in = TRUE"
	}
	query += " ORDER BY created_at DESC"

	rows, err := ac.db.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load user emails"})
		return
	}
	defer rows.Close()

	type emailRecord struct {
		Email                string `json:"email"`
		Name                 string `json:"name,omitempty"`
		MarketingOptIn       bool   `json:"marketing_opt_in"`
		SignupPlanPreference string `json:"signup_plan_preference,omitempty"`
	}

	var records []emailRecord
	for rows.Next() {
		var (
			email          string
			name           sql.NullString
			optIn          bool
			planPreference sql.NullString
		)
		if err := rows.Scan(&email, &name, &optIn, &planPreference); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse user record"})
			return
		}
		record := emailRecord{
			Email:          email,
			MarketingOptIn: optIn,
		}
		if name.Valid {
			record.Name = name.String
		}
		if planPreference.Valid {
			record.SignupPlanPreference = planPreference.String
		}
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read user emails"})
		return
	}

	if format == "csv" {
		var builder strings.Builder
		writer := csv.NewWriter(&builder)
		_ = writer.Write([]string{"email", "name", "marketing_opt_in", "signup_plan_preference"})
		for _, record := range records {
			row := []string{
				record.Email,
				record.Name,
				fmt.Sprintf("%t", record.MarketingOptIn),
				record.SignupPlanPreference,
			}
			_ = writer.Write(row)
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build CSV"})
			return
		}

		filename := "user-emails-opted-in.csv"
		if includeAll {
			filename = "user-emails-all.csv"
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
		c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(builder.String()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"include_all": includeAll,
		"count":       len(records),
		"users":       records,
	})
}

func (ac *AdminController) GetExitSummary(c *gin.Context) {
	days := 14
	if rawDays := strings.TrimSpace(c.Query("days")); rawDays != "" {
		parsed, err := strconv.Atoi(rawDays)
		if err != nil || parsed < 1 || parsed > 365 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "days must be between 1 and 365"})
			return
		}
		days = parsed
	}

	pageLimit := 10
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 50 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 50"})
			return
		}
		pageLimit = parsed
	}

	homeReferrerLimit := 10
	if rawHomeLimit := strings.TrimSpace(c.Query("home_referrer_limit")); rawHomeLimit != "" {
		parsed, err := strconv.Atoi(rawHomeLimit)
		if err != nil || parsed < 1 || parsed > 50 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "home_referrer_limit must be between 1 and 50"})
			return
		}
		homeReferrerLimit = parsed
	}

	now := time.Now()
	since := now.AddDate(0, 0, -days)

	const pageSummaryQuery = `
SELECT
    page_path,
    COUNT(*) AS exit_count,
    AVG(NULLIF(session_duration_ms, 0)) AS avg_session_ms,
    AVG(NULLIF(page_duration_ms, 0)) AS avg_page_ms,
    AVG(NULLIF(last_step_delta_ms, 0)) AS avg_last_step_ms,
    SUM(CASE WHEN referrer IS NULL OR TRIM(referrer) = '' THEN 1 ELSE 0 END) AS direct_exits
FROM exit_events
WHERE created_at >= $1
GROUP BY page_path
ORDER BY exit_count DESC
LIMIT $2`

	rows, err := ac.db.Query(pageSummaryQuery, since, pageLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load exit summary"})
		return
	}
	defer rows.Close()

	type reasonEntry struct {
		Reason string  `json:"reason"`
		Share  float64 `json:"share"`
		Count  int     `json:"count"`
	}

	type pageSummary struct {
		PagePath           string        `json:"page_path"`
		ExitCount          int           `json:"exit_count"`
		AvgSessionSeconds  float64       `json:"avg_session_seconds"`
		AvgPageSeconds     float64       `json:"avg_page_seconds"`
		AvgLastStepSeconds float64       `json:"avg_last_step_seconds"`
		DirectExitRate     float64       `json:"direct_exit_rate"`
		TopReasons         []reasonEntry `json:"top_reasons"`
	}

	var (
		pageSummaries []pageSummary
		pagePaths     []string
		totalExits    int
	)

	for rows.Next() {
		var (
			pagePath        string
			exitCount       int
			avgSessionMs    sql.NullFloat64
			avgPageMs       sql.NullFloat64
			avgLastStepMs   sql.NullFloat64
			directExitCount sql.NullInt64
		)

		if err := rows.Scan(
			&pagePath,
			&exitCount,
			&avgSessionMs,
			&avgPageMs,
			&avgLastStepMs,
			&directExitCount,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse exit summary"})
			return
		}

		summary := pageSummary{
			PagePath:  pagePath,
			ExitCount: exitCount,
		}

		if avgSessionMs.Valid && avgSessionMs.Float64 > 0 {
			summary.AvgSessionSeconds = roundToOneDecimal(avgSessionMs.Float64 / 1000)
		}
		if avgPageMs.Valid && avgPageMs.Float64 > 0 {
			summary.AvgPageSeconds = roundToOneDecimal(avgPageMs.Float64 / 1000)
		}
		if avgLastStepMs.Valid && avgLastStepMs.Float64 > 0 {
			summary.AvgLastStepSeconds = roundToOneDecimal(avgLastStepMs.Float64 / 1000)
		}
		if directExitCount.Valid && exitCount > 0 && directExitCount.Int64 > 0 {
			summary.DirectExitRate = roundToOneDecimal(float64(directExitCount.Int64) / float64(exitCount) * 100)
		}

		pageSummaries = append(pageSummaries, summary)
		pagePaths = append(pagePaths, pagePath)
		totalExits += exitCount
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read exit summary"})
		return
	}

	if len(pageSummaries) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"since":          since,
			"until":          now,
			"total_exits":    0,
			"unique_pages":   0,
			"page_summaries": []pageSummary{},
			"home_referrers": []gin.H{},
		})
		return
	}

	const reasonQuery = `
WITH reason_counts AS (
    SELECT
        page_path,
        COALESCE(NULLIF(TRIM(reason), ''), 'unknown') AS reason,
        COUNT(*) AS reason_count
    FROM exit_events
    WHERE created_at >= $1
      AND page_path = ANY($2)
    GROUP BY page_path, reason
)
SELECT
    page_path,
    reason,
    reason_count,
    ROW_NUMBER() OVER (PARTITION BY page_path ORDER BY reason_count DESC) AS reason_rank,
    SUM(reason_count) OVER (PARTITION BY page_path) AS total_for_page
FROM reason_counts
ORDER BY page_path, reason_rank`

	reasonRows, err := ac.db.Query(reasonQuery, since, pq.Array(pagePaths))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load exit reasons"})
		return
	}
	defer reasonRows.Close()

	const topReasonLimit = 3

	type reasonAggregate struct {
		Reason       string
		Count        int
		Rank         int
		TotalForPage int
	}

	reasons := make(map[string][]reasonAggregate)
	for reasonRows.Next() {
		var (
			pagePath     string
			reason       string
			count        int
			rank         int
			totalForPage int
		)
		if err := reasonRows.Scan(&pagePath, &reason, &count, &rank, &totalForPage); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse exit reasons"})
			return
		}
		if rank <= topReasonLimit {
			reasons[pagePath] = append(reasons[pagePath], reasonAggregate{
				Reason:       reason,
				Count:        count,
				Rank:         rank,
				TotalForPage: totalForPage,
			})
		}
	}
	if err := reasonRows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read exit reasons"})
		return
	}

	for i, summary := range pageSummaries {
		if reasonAggs, found := reasons[summary.PagePath]; found {
			var entries []reasonEntry
			for _, agg := range reasonAggs {
				share := 0.0
				if agg.TotalForPage > 0 {
					share = roundToOneDecimal(float64(agg.Count) / float64(agg.TotalForPage) * 100)
				}
				entries = append(entries, reasonEntry{
					Reason: agg.Reason,
					Share:  share,
					Count:  agg.Count,
				})
			}
			pageSummaries[i].TopReasons = entries
		}
	}

	const homeReferrerQuery = `
SELECT
    COALESCE(NULLIF(TRIM(referrer), ''), 'direct') AS referrer,
    COUNT(*) AS exit_count,
    AVG(NULLIF(page_duration_ms, 0)) AS avg_page_ms,
    AVG(NULLIF(session_duration_ms, 0)) AS avg_session_ms
FROM exit_events
WHERE created_at >= $1 AND page_path = $2
GROUP BY referrer
ORDER BY exit_count DESC
LIMIT $3`

	refRows, err := ac.db.Query(homeReferrerQuery, since, "/", homeReferrerLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load home referrers"})
		return
	}
	defer refRows.Close()

	type referrerSummary struct {
		Referrer          string  `json:"referrer"`
		ExitCount         int     `json:"exit_count"`
		AvgPageSeconds    float64 `json:"avg_page_seconds"`
		AvgSessionSeconds float64 `json:"avg_session_seconds"`
	}

	var referrers []referrerSummary
	for refRows.Next() {
		var (
			referrer     string
			exitCount    int
			avgPageMs    sql.NullFloat64
			avgSessionMs sql.NullFloat64
		)
		if err := refRows.Scan(&referrer, &exitCount, &avgPageMs, &avgSessionMs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse home referrers"})
			return
		}

		summary := referrerSummary{
			Referrer:  referrer,
			ExitCount: exitCount,
		}
		if avgPageMs.Valid && avgPageMs.Float64 > 0 {
			summary.AvgPageSeconds = roundToOneDecimal(avgPageMs.Float64 / 1000)
		}
		if avgSessionMs.Valid && avgSessionMs.Float64 > 0 {
			summary.AvgSessionSeconds = roundToOneDecimal(avgSessionMs.Float64 / 1000)
		}

		referrers = append(referrers, summary)
	}
	if err := refRows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read home referrers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"since":          since,
		"until":          now,
		"total_exits":    totalExits,
		"unique_pages":   len(pageSummaries),
		"page_summaries": pageSummaries,
		"home_referrers": referrers,
	})
}

func roundToOneDecimal(value float64) float64 {
	return math.Round(value*10) / 10
}

func (ac *AdminController) scanMembership(scanner rowScanner) (gin.H, error) {
	var (
		id                 int
		email              string
		name               sql.NullString
		isAdmin            bool
		planID             sql.NullInt64
		planName           sql.NullString
		planDisplay        sql.NullString
		userStatus         sql.NullString
		billingStatus      sql.NullString
		resumeLimit        sql.NullInt64
		resumePeriod       sql.NullString
		trialEnd           sql.NullTime
		stripeCustomerID   sql.NullString
		stripeSubscription sql.NullString
		periodStart        sql.NullTime
		periodEnd          sql.NullTime
		cancelAtPeriodEnd  bool
		marketingOptIn     bool
		signupPlanPref     sql.NullString
		createdAt          time.Time
		updatedAt          time.Time
	)

	if err := scanner.Scan(
		&id,
		&email,
		&name,
		&isAdmin,
		&planID,
		&planName,
		&planDisplay,
		&userStatus,
		&billingStatus,
		&resumeLimit,
		&resumePeriod,
		&trialEnd,
		&stripeCustomerID,
		&stripeSubscription,
		&periodStart,
		&periodEnd,
		&cancelAtPeriodEnd,
		&marketingOptIn,
		&signupPlanPref,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}

	toString := func(ns sql.NullString) string {
		if ns.Valid {
			return ns.String
		}
		return ""
	}

	toInt := func(ni sql.NullInt64) interface{} {
		if ni.Valid {
			return int(ni.Int64)
		}
		return nil
	}

	toTimeValue := func(nt sql.NullTime) interface{} {
		if nt.Valid {
			return nt.Time
		}
		return nil
	}

	billing := toString(billingStatus)
	if billing == "" {
		billing = toString(userStatus)
	}

	return gin.H{
		"id":                     id,
		"email":                  email,
		"name":                   toString(name),
		"is_admin":               isAdmin,
		"plan_id":                toInt(planID),
		"plan_name":              toString(planName),
		"plan_display_name":      toString(planDisplay),
		"subscription_status":    toString(userStatus),
		"billing_status":         billing,
		"resume_limit":           toInt(resumeLimit),
		"resume_period":          toString(resumePeriod),
		"trial_end_date":         toTimeValue(trialEnd),
		"stripe_customer_id":     toString(stripeCustomerID),
		"stripe_subscription_id": toString(stripeSubscription),
		"current_period_start":   toTimeValue(periodStart),
		"current_period_end":     toTimeValue(periodEnd),
		"cancel_at_period_end":   cancelAtPeriodEnd,
		"marketing_opt_in":       marketingOptIn,
		"signup_plan_preference": toString(signupPlanPref),
		"created_at":             createdAt,
		"updated_at":             updatedAt,
	}, nil
}
