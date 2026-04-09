package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"resumeai/models"
	"resumeai/utils"
)

type AshbyProvider struct {
	client *http.Client
	logger *utils.Logger
}

func NewAshbyProvider(client *http.Client, logger *utils.Logger) *AshbyProvider {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if logger == nil {
		logger = utils.NewLogger()
	}
	return &AshbyProvider{client: client, logger: logger}
}

func (p *AshbyProvider) FetchJobs(ctx context.Context, company *models.JobCompany) ([]*models.JobPosting, error) {
	slug, err := p.resolveOrgSlug(company)
	if err != nil {
		return nil, err
	}

	// Ashby migrated to GraphQL API — the REST endpoint no longer works
	return p.fetchJobsGraphQL(ctx, company.ID, slug)
}

// fetchJobsGraphQL uses Ashby's non-user GraphQL API to fetch job postings.
func (p *AshbyProvider) fetchJobsGraphQL(ctx context.Context, companyID int, slug string) ([]*models.JobPosting, error) {
	gqlQuery := `query ApiJobBoardWithTeams($organizationHostedJobsPageName: String!) {
		jobBoard: jobBoardWithTeams(organizationHostedJobsPageName: $organizationHostedJobsPageName) {
			teams { id name }
			jobPostings { id title teamId locationName }
		}
	}`

	reqBody := map[string]interface{}{
		"operationName": "ApiJobBoardWithTeams",
		"variables":     map[string]string{"organizationHostedJobsPageName": slug},
		"query":         gqlQuery,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal graphql request: %w", err)
	}

	endpoint := "https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiJobBoardWithTeams"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ashby returned status %d", resp.StatusCode)
	}

	var gqlResp struct {
		Data struct {
			JobBoard *struct {
				Teams       []ashbyGQLTeam `json:"teams"`
				JobPostings []ashbyGQLJob  `json:"jobPostings"`
			} `json:"jobBoard"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return nil, fmt.Errorf("parse ashby graphql response: %w", err)
	}

	if gqlResp.Data.JobBoard == nil {
		return nil, fmt.Errorf("ashby returned status 404")
	}

	// Build team ID → name lookup
	teamMap := make(map[string]string)
	for _, t := range gqlResp.Data.JobBoard.Teams {
		teamMap[t.ID] = t.Name
	}

	results := make([]*models.JobPosting, 0, len(gqlResp.Data.JobBoard.JobPostings))
	for _, job := range gqlResp.Data.JobBoard.JobPostings {
		posting := p.transformGQLJob(companyID, slug, job, teamMap)
		if posting != nil {
			results = append(results, posting)
		}
	}

	return results, nil
}

type ashbyGQLTeam struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ashbyGQLJob struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	TeamID       string `json:"teamId"`
	LocationName string `json:"locationName"`
}

func (p *AshbyProvider) transformGQLJob(companyID int, slug string, job ashbyGQLJob, teamMap map[string]string) *models.JobPosting {
	if strings.TrimSpace(job.ID) == "" || strings.TrimSpace(job.Title) == "" {
		return nil
	}

	location := strings.TrimSpace(job.LocationName)
	remote := ""
	if strings.Contains(strings.ToLower(location), "remote") {
		remote = "Remote"
	}

	department := teamMap[job.TeamID]
	jobURL := fmt.Sprintf("https://jobs.ashbyhq.com/%s/%s", url.PathEscape(slug), url.PathEscape(job.ID))

	raw, _ := json.Marshal(job)

	return &models.JobPosting{
		CompanyID:      companyID,
		ExternalJobID:  strings.TrimSpace(job.ID),
		Title:          strings.TrimSpace(job.Title),
		Location:       location,
		RemoteType:     remote,
		Department:     department,
		JobURL:         jobURL,
		ApplicationURL: jobURL,
		Description:    job.Title,
		RawPayload:     raw,
	}
}

func (p *AshbyProvider) resolveOrgSlug(company *models.JobCompany) (string, error) {
	if company.ExternalIdentifier != nil && strings.TrimSpace(*company.ExternalIdentifier) != "" {
		return strings.TrimSpace(*company.ExternalIdentifier), nil
	}

	if company.CareersURL == "" {
		return "", fmt.Errorf("ashby org slug missing for %s", company.Name)
	}

	parsed, err := url.Parse(company.CareersURL)
	if err != nil {
		return "", fmt.Errorf("invalid careers_url for %s: %w", company.Name, err)
	}

	segments := strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' })
	for _, segment := range segments {
		if strings.EqualFold(segment, "jobs") {
			continue
		}
		if segment != "" {
			return strings.TrimSpace(segment), nil
		}
	}

	return "", fmt.Errorf("could not derive ashby org slug from %s", company.CareersURL)
}

