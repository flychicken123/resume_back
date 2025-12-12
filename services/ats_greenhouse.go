package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"resumeai/models"
	"resumeai/utils"
)

type GreenhouseProvider struct {
	client *http.Client
	logger *utils.Logger
}

type greenhouseJobResponse struct {
	Jobs []greenhouseJob `json:"jobs"`
}

type greenhouseJob struct {
	ID          int                `json:"id"`
	Title       string             `json:"title"`
	UpdatedAt   string             `json:"updated_at"`
	CreatedAt   string             `json:"created_at"`
	AbsoluteURL string             `json:"absolute_url"`
	ApplyURL    string             `json:"apply_url"`
	Location    greenhouseLocation `json:"location"`
	Metadata    []greenhouseMeta   `json:"metadata"`
	Departments []greenhouseLabel  `json:"departments"`
	Offices     []greenhouseLabel  `json:"offices"`
	Content     string             `json:"content"`
}

type greenhouseLocation struct {
	Name string `json:"name"`
}

type greenhouseMeta struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

type greenhouseLabel struct {
	Name string `json:"name"`
}

func NewGreenhouseProvider(client *http.Client, logger *utils.Logger) *GreenhouseProvider {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &GreenhouseProvider{client: client, logger: logger}
}

func (p *GreenhouseProvider) FetchJobs(ctx context.Context, company *models.JobCompany) ([]*models.JobPosting, error) {
	token, err := p.resolveBoardToken(company)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true&per_page=500", url.PathEscape(token))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("greenhouse returned status %d", resp.StatusCode)
	}

	var payload greenhouseJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse greenhouse response: %w", err)
	}

	results := make([]*models.JobPosting, 0, len(payload.Jobs))
	for _, job := range payload.Jobs {
		posting, err := p.transformJob(company.ID, job)
		if err != nil {
			p.logger.Warn("skipping greenhouse job", map[string]interface{}{"company_id": company.ID, "error": err.Error()})
			continue
		}
		results = append(results, posting)
	}
	return results, nil
}

func (p *GreenhouseProvider) resolveBoardToken(company *models.JobCompany) (string, error) {
	if company.ExternalIdentifier != nil && strings.TrimSpace(*company.ExternalIdentifier) != "" {
		return strings.TrimSpace(*company.ExternalIdentifier), nil
	}
	if company.CareersURL != "" {
		if parsed, err := url.Parse(company.CareersURL); err == nil {
			query := parsed.Query()
			for _, key := range []string{"for", "board", "token", "company"} {
				if token := strings.TrimSpace(query.Get(key)); token != "" {
					return token, nil
				}
			}

			segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			for i := 0; i < len(segments); i++ {
				if strings.EqualFold(segments[i], "jobs") && i > 0 {
					if token := strings.TrimSpace(segments[i-1]); token != "" {
						return token, nil
					}
				}
			}

			for i := len(segments) - 1; i >= 0; i-- {
				token := strings.TrimSpace(segments[i])
				if token == "" {
					continue
				}
				switch strings.ToLower(token) {
				case "jobs", "job", "embed", "job_board":
					continue
				default:
					return token, nil
				}
			}
		}
	}
	return "", errors.New("greenhouse board token is required for company")
}

func (p *GreenhouseProvider) transformJob(companyID int, job greenhouseJob) (*models.JobPosting, error) {
	if job.ID == 0 || strings.TrimSpace(job.Title) == "" {
		return nil, errors.New("missing job id or title")
	}

	location := strings.TrimSpace(job.Location.Name)
	department := joinLabels(job.Departments)
	remoteType := deriveRemoteType(location, job.Metadata)
	employmentType := metadataLookup(job.Metadata, []string{"employment type", "job type"})

	var postedAt *time.Time
	if ts := parseGreenhouseTime(job.CreatedAt); ts != nil {
		postedAt = ts
	} else if ts := parseGreenhouseTime(job.UpdatedAt); ts != nil {
		postedAt = ts
	}

	raw, _ := json.Marshal(job)

	return &models.JobPosting{
		CompanyID:      companyID,
		ExternalJobID:  fmt.Sprintf("%d", job.ID),
		Title:          strings.TrimSpace(job.Title),
		Location:       location,
		RemoteType:     remoteType,
		Department:     department,
		EmploymentType: employmentType,
		JobURL:         strings.TrimSpace(job.AbsoluteURL),
		ApplicationURL: strings.TrimSpace(job.ApplyURL),
		Description:    strings.TrimSpace(job.Content),
		PostedAt:       postedAt,
		RawPayload:     raw,
	}, nil
}

func parseGreenhouseTime(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	formats := []string{time.RFC3339, "2006-01-02T15:04:05-07:00"}
	for _, layout := range formats {
		if ts, err := time.Parse(layout, trimmed); err == nil {
			return &ts
		}
	}
	return nil
}

func metadataLookup(metadata []greenhouseMeta, keys []string) string {
	if len(metadata) == 0 {
		return ""
	}
	for _, key := range keys {
		for _, meta := range metadata {
			if strings.EqualFold(meta.Name, key) {
				return strings.TrimSpace(fmt.Sprint(meta.Value))
			}
		}
	}
	return ""
}

func joinLabels(labels []greenhouseLabel) string {
	if len(labels) == 0 {
		return ""
	}
	values := make([]string, 0, len(labels))
	for _, l := range labels {
		if trimmed := strings.TrimSpace(l.Name); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return strings.Join(values, ", ")
}

func deriveRemoteType(location string, metadata []greenhouseMeta) string {
	lowerLoc := strings.ToLower(location)
	if strings.Contains(lowerLoc, "remote") {
		return "Remote"
	}
	if val := metadataLookup(metadata, []string{"remote", "work arrangement", "workplace type"}); val != "" {
		return val
	}
	return ""
}
