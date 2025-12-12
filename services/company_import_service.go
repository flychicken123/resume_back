package services

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"resumeai/models"
)

// ErrCompanyImportInvalidFormat indicates the uploaded CSV is missing required headers or is unreadable.
var ErrCompanyImportInvalidFormat = errors.New("invalid company import format")

// CompanyImportError captures details for a row that could not be imported.
type CompanyImportError struct {
	Row     int               `json:"row"`
	Message string            `json:"message"`
	Values  map[string]string `json:"values,omitempty"`
}

// CompanyImportSummary reports the outcome of a CSV import operation.
type CompanyImportSummary struct {
	TotalRows int                  `json:"total_rows"`
	Processed int                  `json:"processed_rows"`
	Inserted  int                  `json:"inserted"`
	Updated   int                  `json:"updated"`
	Skipped   int                  `json:"skipped"`
	Errors    []CompanyImportError `json:"errors"`
}

// CompanyImportService coordinates CSV parsing and upserting JobCompany records.
type CompanyImportService struct {
	companies       *models.JobCompanyModel
	maxErrorDetails int
}

// NewCompanyImportService constructs a CompanyImportService.
func NewCompanyImportService(companies *models.JobCompanyModel) *CompanyImportService {
	return &CompanyImportService{
		companies:       companies,
		maxErrorDetails: 25,
	}
}

// ImportCSV reads a CSV file from reader, validates rows, and upserts job companies.
func (s *CompanyImportService) ImportCSV(reader io.Reader) (*CompanyImportSummary, error) {
	csvReader := csv.NewReader(bufio.NewReader(reader))
	csvReader.TrimLeadingSpace = true

	headers, err := csvReader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return &CompanyImportSummary{}, nil
		}
		return nil, fmt.Errorf("%w: unable to read header row: %v", ErrCompanyImportInvalidFormat, err)
	}

	fieldIndex := map[string]int{}
	for idx, raw := range headers {
		key := normalizeHeader(raw)
		if key != "" {
			fieldIndex[key] = idx
		}
	}

	required := []string{"name", "careers_url"}
	for _, field := range required {
		if _, ok := fieldIndex[field]; !ok {
			return nil, fmt.Errorf("%w: missing required column '%s'", ErrCompanyImportInvalidFormat, field)
		}
	}

	summary := &CompanyImportSummary{}
	validCompanies := make([]*models.JobCompany, 0, 128)

	for {
		row, err := csvReader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			s.recordError(summary, summary.TotalRows+2, fmt.Errorf("failed to read row: %v", err), nil)
			summary.Skipped++
			summary.TotalRows++
			continue
		}

		summary.TotalRows++
		values := extractValues(row, fieldIndex)
		company, err := buildCompanyFromValues(values)
		if err != nil {
			summary.Skipped++
			s.recordError(summary, summary.TotalRows+1, err, values)
			continue
		}

		validCompanies = append(validCompanies, company)
		summary.Processed++
	}

	if len(validCompanies) == 0 {
		return summary, nil
	}

	inserted, updated, err := s.companies.BulkUpsert(validCompanies)
	if err != nil {
		return nil, err
	}

	summary.Inserted = inserted
	summary.Updated = updated
	return summary, nil
}

func (s *CompanyImportService) recordError(summary *CompanyImportSummary, row int, err error, values map[string]string) {
	if len(summary.Errors) >= s.maxErrorDetails {
		return
	}
	entry := CompanyImportError{
		Row:     row,
		Message: err.Error(),
	}
	if len(values) > 0 {
		entry.Values = values
	}
	summary.Errors = append(summary.Errors, entry)
}

func normalizeHeader(raw string) string {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	switch trimmed {
	case "company", "company_name", "name":
		return "name"
	case "website", "website_url", "site":
		return "website_url"
	case "careers", "careers_url", "ats_url", "job_board", "job_board_url":
		return "careers_url"
	case "ats", "ats_provider", "provider":
		return "ats_provider"
	case "external_id", "external_identifier", "ticker":
		return "external_identifier"
	case "sync_interval", "sync_interval_minutes", "interval_minutes":
		return "sync_interval_minutes"
	case "active", "is_active", "enabled":
		return "is_active"
	default:
		return trimmed
	}
}

func extractValues(row []string, index map[string]int) map[string]string {
	values := make(map[string]string, len(index))
	for key, idx := range index {
		if idx >= 0 && idx < len(row) {
			values[key] = strings.TrimSpace(row[idx])
		}
	}
	return values
}

func buildCompanyFromValues(values map[string]string) (*models.JobCompany, error) {
	name := strings.TrimSpace(values["name"])
	careers := strings.TrimSpace(values["careers_url"])
	provider := strings.TrimSpace(values["ats_provider"])

	if name == "" {
		return nil, fmt.Errorf("missing company name")
	}
	if careers == "" {
		return nil, fmt.Errorf("missing careers_url")
	}

	if provider == "" || strings.EqualFold(provider, "auto") {
		provider = DetectATSProvider(careers)
	}
	if provider == "" {
		return nil, fmt.Errorf("missing ats_provider and unable to infer from careers_url")
	}
	provider = strings.ToLower(provider)

	website := strings.TrimSpace(values["website_url"])

	syncInterval := 1440
	if raw := strings.TrimSpace(values["sync_interval_minutes"]); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			syncInterval = parsed
		} else {
			return nil, fmt.Errorf("invalid sync_interval_minutes: %s", raw)
		}
	}

	isActive := true
	if raw := strings.TrimSpace(strings.ToLower(values["is_active"])); raw != "" {
		switch raw {
		case "true", "1", "yes", "y":
			isActive = true
		case "false", "0", "no", "n":
			isActive = false
		default:
			return nil, fmt.Errorf("invalid is_active value: %s", raw)
		}
	}

	company := &models.JobCompany{
		Name:                name,
		WebsiteURL:          website,
		CareersURL:          careers,
		ATSProvider:         provider,
		IsActive:            isActive,
		SyncIntervalMinutes: syncInterval,
	}

	if ext := strings.TrimSpace(values["external_identifier"]); ext != "" {
		company.ExternalIdentifier = &ext
	}

	return company, nil
}
