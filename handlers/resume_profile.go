package handlers

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/gin-gonic/gin"

	"resumeai/models"
)

// ResumeProfileStore allows handlers to persist the latest resume snapshot for a user.
type ResumeProfileStore interface {
	SaveProfile(ctx context.Context, userID int, input models.ResumeProfileSaveInput) (*models.Resume, error)
}

var resumeProfileStore ResumeProfileStore

func SetResumeProfileStore(store ResumeProfileStore) {
	resumeProfileStore = store
}

func persistResumeProfileFromRequest(c *gin.Context, req ResumeRequest) error {
	if resumeProfileStore == nil {
		return nil
	}

	userID, ok := extractUserID(c)
	if !ok {
		return nil
	}

	input := buildProfileSaveInputFromRequest(req)
	_, err := resumeProfileStore.SaveProfile(c.Request.Context(), userID, input)
	if err != nil {
		log.Printf("failed to save resume profile for user %d: %v", userID, err)
	}
	return err
}

func persistResumeProfileFromJSON(c *gin.Context, raw json.RawMessage, fallback models.ResumeProfileSaveInput) error {
	if resumeProfileStore == nil {
		return nil
	}

	userID, ok := extractUserID(c)
	if !ok {
		return nil
	}

	fallback.SelectedFormat = normalizeTemplateFormat(fallback.SelectedFormat)
	if len(raw) > 0 {
		fallback.ProfileJSON = raw
		models.MergeProfileFieldsFromJSON(raw, &fallback)
	}

	_, err := resumeProfileStore.SaveProfile(c.Request.Context(), userID, fallback)
	if err != nil {
		log.Printf("failed to save resume profile for user %d: %v", userID, err)
	}
	return err
}

func buildProfileSaveInputFromRequest(req ResumeRequest) models.ResumeProfileSaveInput {
	normalizedFormat := normalizeTemplateFormat(req.Format)
	profileJSON := buildProfileJSONFromRequest(req, normalizedFormat)

	experience := strings.TrimSpace(req.Experience)
	if experience == "" && len(req.Experiences) > 0 {
		if raw, err := json.Marshal(req.Experiences); err == nil {
			experience = strings.TrimSpace(string(raw))
		}
	}

	input := models.ResumeProfileSaveInput{
		Name:              req.Name,
		Email:             req.Email,
		Phone:             req.Phone,
		Summary:           req.Summary,
		Experience:        experience,
		Education:         req.Education,
		JobDescription:    req.JobDescription,
		Location:          req.Location,
		Skills:            req.Skills,
		SkillsCategorized: req.SkillsCategorized,
		SelectedFormat:    normalizedFormat,
		ProfileJSON:       profileJSON,
	}
	models.MergeProfileFieldsFromJSON(profileJSON, &input)
	return input
}

func buildProfileJSONFromRequest(req ResumeRequest, normalizedFormat string) json.RawMessage {
	if len(req.ResumeData) > 0 && json.Valid(req.ResumeData) {
		return req.ResumeData
	}

	payload := map[string]interface{}{
		"name":              req.Name,
		"email":             req.Email,
		"phone":             req.Phone,
		"summary":           req.Summary,
		"experience":        req.Experience,
		"education":         req.Education,
		"jobDescription":    req.JobDescription,
		"location":          req.Location,
		"skills":            req.Skills,
		"skillsCategorized": req.SkillsCategorized,
		"format":            normalizedFormat,
		"engine":            req.Engine,
	}
	if req.Experiences != nil {
		payload["experiences"] = req.Experiences
	}
	if req.HtmlContent != "" {
		payload["htmlContent"] = req.HtmlContent
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return raw
}
