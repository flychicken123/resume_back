package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"resumeai/models"
	"resumeai/utils"
)

type LeverProvider struct {
	client *http.Client
	logger *utils.Logger
}

type leverJob struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	HostedURL  string `json:"hostedUrl"`
	ApplyURL   string `json:"applyUrl"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
	Categories struct {
		Location   string `json:"location"`
		Team       string `json:"team"`
		Department string `json:"department"`
		Commitment string `json:"commitment"`
	} `json:"categories"`
	Description          string `json:"description"`
	DescriptionPlain     string `json:"descriptionPlain"`
	DescriptionBodyPlain string `json:"descriptionBodyPlain"`
}

func NewLeverProvider(client *http.Client, logger *utils.Logger) *LeverProvider {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &LeverProvider{client: client, logger: logger}
}

func (p *LeverProvider) FetchJobs(ctx context.Context, company *models.JobCompany) ([]*models.JobPosting, error) {
	slug, err := p.resolveCompanySlug(company)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json&limit=500", url.PathEscape(slug))
	resp, err := p.doFetchWithRetry(ctx, endpoint, slug, company.ID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusNotFound {
			p.logger.Warn("lever slug not found, returning no jobs", map[string]interface{}{"slug": slug, "company_id": company.ID})
			return []*models.JobPosting{}, nil
		}
		return nil, fmt.Errorf("lever returned status %d for slug %s", resp.StatusCode, slug)
	}

	var payload []leverJob
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse lever response: %w", err)
	}

	results := make([]*models.JobPosting, 0, len(payload))
	for _, job := range payload {
		posting, err := p.transformJob(company.ID, slug, job)
		if err != nil {
			p.logger.Warn("skipping lever job", map[string]interface{}{"company_id": company.ID, "error": err.Error()})
			continue
		}
		results = append(results, posting)
	}

	return results, nil
}

func (p *LeverProvider) doFetchWithRetry(ctx context.Context, endpoint, slug string, companyID int) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "HiHiredJobSync/1.0 (+https://hihired.org)")

		resp, err := p.client.Do(req)
		if err == nil && resp != nil && !isRetryableLeverStatus(resp.StatusCode) {
			return resp, nil
		}

		if resp != nil {
			lastErr = fmt.Errorf("lever returned status %d for slug %s", resp.StatusCode, slug)
			resp.Body.Close()
		} else {
			lastErr = err
		}

		if !isRetryableLeverError(lastErr) && (resp == nil || !isRetryableLeverStatus(resp.StatusCode)) {
			return nil, lastErr
		}
		if attempt == 3 {
			break
		}

		p.logger.Warn("lever fetch retry", map[string]interface{}{"slug": slug, "company_id": companyID, "attempt": attempt, "error": lastErr.Error()})
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 1500 * time.Millisecond):
		}
	}
	return nil, lastErr
}

func isRetryableLeverStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusInternalServerError || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func isRetryableLeverError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "connection reset")
}

func (p *LeverProvider) resolveCompanySlug(company *models.JobCompany) (string, error) {
	if company.ExternalIdentifier != nil && strings.TrimSpace(*company.ExternalIdentifier) != "" {
		return strings.TrimSpace(*company.ExternalIdentifier), nil
	}

	if company.CareersURL == "" {
		return "", fmt.Errorf("lever company slug missing for %s", company.Name)
	}

	parsed, err := url.Parse(company.CareersURL)
	if err != nil {
		return "", fmt.Errorf("invalid careers_url for %s: %w", company.Name, err)
	}

	segments := strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' })
	if len(segments) == 0 {
		return "", fmt.Errorf("could not derive lever slug from %s", company.CareersURL)
	}

	slug := strings.TrimSpace(segments[0])
	if slug == "view" && len(segments) > 1 {
		slug = strings.TrimSpace(segments[1])
	}
	if slug == "" {
		return "", fmt.Errorf("could not derive lever slug from %s", company.CareersURL)
	}

	return slug, nil
}

func (p *LeverProvider) transformJob(companyID int, companySlug string, job leverJob) (*models.JobPosting, error) {
	if strings.TrimSpace(job.ID) == "" || strings.TrimSpace(job.Text) == "" {
		return nil, fmt.Errorf("missing job id or title")
	}

	location := strings.TrimSpace(job.Categories.Location)
	department := firstNonEmpty(strings.TrimSpace(job.Categories.Team), strings.TrimSpace(job.Categories.Department))
	employmentType := strings.TrimSpace(job.Categories.Commitment)
	remote := deriveLeverRemote(location)
	description := firstNonEmpty(
		strings.TrimSpace(job.DescriptionBodyPlain),
		strings.TrimSpace(job.DescriptionPlain),
		strings.TrimSpace(job.Description),
	)

	postedAt := parseLeverTime(job.CreatedAt)
	jobURL := firstNonEmpty(strings.TrimSpace(job.HostedURL), fmt.Sprintf("https://jobs.lever.co/%s/%s", url.PathEscape(companySlug), url.PathEscape(job.ID)))
	applyURL := strings.TrimSpace(job.ApplyURL)
	if applyURL == "" {
		applyURL = jobURL
	}

	raw, _ := json.Marshal(job)

	return &models.JobPosting{
		CompanyID:      companyID,
		ExternalJobID:  strings.TrimSpace(job.ID),
		Title:          strings.TrimSpace(job.Text),
		Location:       location,
		RemoteType:     remote,
		Department:     department,
		EmploymentType: employmentType,
		JobURL:         jobURL,
		ApplicationURL: applyURL,
		Description:    description,
		PostedAt:       postedAt,
		RawPayload:     raw,
	}, nil
}

func deriveLeverRemote(location string) string {
	if strings.Contains(strings.ToLower(location), "remote") {
		return "Remote"
	}
	return ""
}

func parseLeverTime(value int64) *time.Time {
	if value <= 0 {
		return nil
	}
	ts := time.UnixMilli(value)
	return &ts
}
