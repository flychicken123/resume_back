package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"resumeai/services"
)

var polishAgent *services.PolishAgent

// SetPolishAgent injects the polish agent dependency.
func SetPolishAgent(agent *services.PolishAgent) {
	polishAgent = agent
}

// PolishRequest represents the request to polish resume content.
type PolishRequest struct {
	Message        string                 `json:"message" binding:"required"`
	ResumeData     map[string]interface{} `json:"resume_data"`
	JobDescription string                 `json:"job_description"`
}

// PolishResponse represents the response with polished content.
type PolishResponse struct {
	Success               bool                   `json:"success"`
	IsPolishRequest       bool                   `json:"isPolishRequest"`
	Section               string                 `json:"section,omitempty"`
	Identifier            string                 `json:"identifier,omitempty"`
	EntryIndex            int                    `json:"entryIndex,omitempty"`
	PolishedContent       interface{}            `json:"polishedContent,omitempty"`
	Message               string                 `json:"message"`
	NeedsClarification    bool                   `json:"needsClarification,omitempty"`
	ClarificationQuestion string                 `json:"clarificationQuestion,omitempty"`
	UpdatedResumeData     map[string]interface{} `json:"updatedResumeData,omitempty"`
}

// PolishResume handles requests to polish/optimize resume sections.
func PolishResume(c *gin.Context) {
	if polishAgent == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Polish agent is not available"})
		return
	}

	var req PolishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	message := strings.TrimSpace(req.Message)
	if message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Message cannot be empty"})
		return
	}

	ctx := c.Request.Context()

	// Detect polish intent
	intent, err := polishAgent.DetectPolishIntent(ctx, message, req.ResumeData, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to analyze request: " + err.Error()})
		return
	}

	// If not a polish request, return early
	if !intent.IsPolishRequest {
		c.JSON(http.StatusOK, PolishResponse{
			Success:         true,
			IsPolishRequest: false,
			Message:         "This doesn't appear to be a polish/optimize request.",
		})
		return
	}

	// Never ask for clarification - always proceed with polishing using existing data
	// If no specific entry identified, we'll polish ALL entries in the section

	// Polish the identified section
	response := PolishResponse{
		Success:         true,
		IsPolishRequest: true,
		Section:         intent.Section,
		Identifier:      intent.Identifier,
		EntryIndex:      intent.EntryIndex,
	}

	// Create a copy of resume data for updates
	updatedData := copyResumeData(req.ResumeData)

	switch intent.Section {
	case "experience":
		polishedContent, msg, err := polishExperienceSection(ctx, intent, req.ResumeData, req.JobDescription, updatedData)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to polish experience: " + err.Error()})
			return
		}
		response.PolishedContent = polishedContent
		response.Message = msg
		response.UpdatedResumeData = updatedData

	case "education":
		polishedContent, msg, err := polishEducationSection(ctx, intent, req.ResumeData, updatedData)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to polish education: " + err.Error()})
			return
		}
		response.PolishedContent = polishedContent
		response.Message = msg
		response.UpdatedResumeData = updatedData

	case "projects":
		polishedContent, msg, err := polishProjectsSection(ctx, intent, req.ResumeData, req.JobDescription, updatedData)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to polish projects: " + err.Error()})
			return
		}
		response.PolishedContent = polishedContent
		response.Message = msg
		response.UpdatedResumeData = updatedData

	case "summary":
		summary, _ := req.ResumeData["summary"].(string)
		polished, err := polishAgent.PolishSummary(ctx, summary, req.ResumeData, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to polish summary: " + err.Error()})
			return
		}
		response.PolishedContent = polished
		response.Message = "Your professional summary has been polished to be more impactful."
		updatedData["summary"] = polished
		response.UpdatedResumeData = updatedData

	case "skills":
		skills, _ := req.ResumeData["skills"].(string)
		polished, err := polishAgent.PolishSkills(ctx, skills, req.JobDescription, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to polish skills: " + err.Error()})
			return
		}
		response.PolishedContent = polished
		response.Message = "Your skills section has been polished and organized."
		updatedData["skills"] = polished
		response.UpdatedResumeData = updatedData

	case "all":
		msg, err := polishAllSections(ctx, req.ResumeData, req.JobDescription, updatedData)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to polish resume: " + err.Error()})
			return
		}
		response.Message = msg
		response.UpdatedResumeData = updatedData

	default:
		response.Message = "I couldn't identify which section to polish. Please specify like 'polish my experience at Microsoft' or 'improve my summary'."
	}

	c.JSON(http.StatusOK, response)
}

func polishExperienceSection(ctx context.Context, intent services.PolishIntent, resumeData map[string]interface{}, jobDesc string, updatedData map[string]interface{}) (interface{}, string, error) {
	experiences, ok := resumeData["experiences"].([]interface{})
	if !ok || len(experiences) == 0 {
		return nil, "No experiences found to polish.", nil
	}

	// If specific entry identified
	if intent.EntryIndex >= 0 && intent.EntryIndex < len(experiences) {
		expMap, ok := experiences[intent.EntryIndex].(map[string]interface{})
		if !ok {
			return nil, "Could not read experience entry.", nil
		}

		polished, err := polishAgent.PolishExperience(ctx, expMap, jobDesc, "")
		if err != nil {
			return nil, "", err
		}

		company, _ := expMap["company"].(string)

		// Update the experiences array
		updatedExperiences := make([]interface{}, len(experiences))
		copy(updatedExperiences, experiences)
		updatedExperiences[intent.EntryIndex] = polished
		updatedData["experiences"] = updatedExperiences

		return polished, "Your experience at " + company + " has been polished with impact-focused statements.", nil
	}

	// Polish all experiences
	polishedExperiences := make([]interface{}, 0, len(experiences))
	for _, exp := range experiences {
		expMap, ok := exp.(map[string]interface{})
		if !ok {
			polishedExperiences = append(polishedExperiences, exp)
			continue
		}

		polished, err := polishAgent.PolishExperience(ctx, expMap, jobDesc, "")
		if err != nil {
			polishedExperiences = append(polishedExperiences, exp)
			continue
		}
		polishedExperiences = append(polishedExperiences, polished)
	}

	updatedData["experiences"] = polishedExperiences
	return polishedExperiences, "All your work experiences have been polished with impact-focused statements.", nil
}

func polishEducationSection(ctx context.Context, intent services.PolishIntent, resumeData map[string]interface{}, updatedData map[string]interface{}) (interface{}, string, error) {
	education, ok := resumeData["education"].([]interface{})
	if !ok || len(education) == 0 {
		return nil, "No education entries found to polish.", nil
	}

	// If specific entry identified
	if intent.EntryIndex >= 0 && intent.EntryIndex < len(education) {
		eduMap, ok := education[intent.EntryIndex].(map[string]interface{})
		if !ok {
			return nil, "Could not read education entry.", nil
		}

		polished, err := polishAgent.PolishEducation(ctx, eduMap, "")
		if err != nil {
			return nil, "", err
		}

		school, _ := eduMap["school"].(string)

		// Update the education array
		updatedEducation := make([]interface{}, len(education))
		copy(updatedEducation, education)
		updatedEducation[intent.EntryIndex] = polished
		updatedData["education"] = updatedEducation

		return polished, "Your education at " + school + " has been polished.", nil
	}

	// Polish all education entries
	polishedEducation := make([]interface{}, 0, len(education))
	for _, edu := range education {
		eduMap, ok := edu.(map[string]interface{})
		if !ok {
			polishedEducation = append(polishedEducation, edu)
			continue
		}

		polished, err := polishAgent.PolishEducation(ctx, eduMap, "")
		if err != nil {
			polishedEducation = append(polishedEducation, edu)
			continue
		}
		polishedEducation = append(polishedEducation, polished)
	}

	updatedData["education"] = polishedEducation
	return polishedEducation, "All your education entries have been polished.", nil
}

func polishProjectsSection(ctx context.Context, intent services.PolishIntent, resumeData map[string]interface{}, jobDesc string, updatedData map[string]interface{}) (interface{}, string, error) {
	projects, ok := resumeData["projects"].([]interface{})
	if !ok || len(projects) == 0 {
		return nil, "No projects found to polish.", nil
	}

	// If specific entry identified
	if intent.EntryIndex >= 0 && intent.EntryIndex < len(projects) {
		projMap, ok := projects[intent.EntryIndex].(map[string]interface{})
		if !ok {
			return nil, "Could not read project entry.", nil
		}

		polished, err := polishAgent.PolishProject(ctx, projMap, jobDesc, "")
		if err != nil {
			return nil, "", err
		}

		projectName, _ := projMap["projectName"].(string)

		// Update the projects array
		updatedProjects := make([]interface{}, len(projects))
		copy(updatedProjects, projects)
		updatedProjects[intent.EntryIndex] = polished
		updatedData["projects"] = updatedProjects

		return polished, "Your project '" + projectName + "' has been polished.", nil
	}

	// Polish all projects
	polishedProjects := make([]interface{}, 0, len(projects))
	for _, proj := range projects {
		projMap, ok := proj.(map[string]interface{})
		if !ok {
			polishedProjects = append(polishedProjects, proj)
			continue
		}

		polished, err := polishAgent.PolishProject(ctx, projMap, jobDesc, "")
		if err != nil {
			polishedProjects = append(polishedProjects, proj)
			continue
		}
		polishedProjects = append(polishedProjects, polished)
	}

	updatedData["projects"] = polishedProjects
	return polishedProjects, "All your projects have been polished.", nil
}

func polishAllSections(ctx context.Context, resumeData map[string]interface{}, jobDesc string, updatedData map[string]interface{}) (string, error) {
	var messages []string

	// Polish experiences
	if experiences, ok := resumeData["experiences"].([]interface{}); ok && len(experiences) > 0 {
		polishedExperiences := make([]interface{}, 0, len(experiences))
		for _, exp := range experiences {
			if expMap, ok := exp.(map[string]interface{}); ok {
				polished, err := polishAgent.PolishExperience(ctx, expMap, jobDesc, "")
				if err != nil {
					polishedExperiences = append(polishedExperiences, exp)
				} else {
					polishedExperiences = append(polishedExperiences, polished)
				}
			} else {
				polishedExperiences = append(polishedExperiences, exp)
			}
		}
		updatedData["experiences"] = polishedExperiences
		messages = append(messages, "experiences")
	}

	// Polish education
	if education, ok := resumeData["education"].([]interface{}); ok && len(education) > 0 {
		polishedEducation := make([]interface{}, 0, len(education))
		for _, edu := range education {
			if eduMap, ok := edu.(map[string]interface{}); ok {
				polished, err := polishAgent.PolishEducation(ctx, eduMap, "")
				if err != nil {
					polishedEducation = append(polishedEducation, edu)
				} else {
					polishedEducation = append(polishedEducation, polished)
				}
			} else {
				polishedEducation = append(polishedEducation, edu)
			}
		}
		updatedData["education"] = polishedEducation
		messages = append(messages, "education")
	}

	// Polish projects
	if projects, ok := resumeData["projects"].([]interface{}); ok && len(projects) > 0 {
		polishedProjects := make([]interface{}, 0, len(projects))
		for _, proj := range projects {
			if projMap, ok := proj.(map[string]interface{}); ok {
				polished, err := polishAgent.PolishProject(ctx, projMap, jobDesc, "")
				if err != nil {
					polishedProjects = append(polishedProjects, proj)
				} else {
					polishedProjects = append(polishedProjects, polished)
				}
			} else {
				polishedProjects = append(polishedProjects, proj)
			}
		}
		updatedData["projects"] = polishedProjects
		messages = append(messages, "projects")
	}

	// Polish summary
	if summary, ok := resumeData["summary"].(string); ok && summary != "" {
		polished, err := polishAgent.PolishSummary(ctx, summary, resumeData, "")
		if err == nil {
			updatedData["summary"] = polished
			messages = append(messages, "summary")
		}
	}

	// Polish skills
	if skills, ok := resumeData["skills"].(string); ok && skills != "" {
		polished, err := polishAgent.PolishSkills(ctx, skills, jobDesc, "")
		if err == nil {
			updatedData["skills"] = polished
			messages = append(messages, "skills")
		}
	}

	if len(messages) == 0 {
		return "No content found to polish.", nil
	}

	return "I've polished your " + strings.Join(messages, ", ") + " to be more impactful and professional.", nil
}

func copyResumeData(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return make(map[string]interface{})
	}

	result := make(map[string]interface{})
	for k, v := range data {
		result[k] = v
	}
	return result
}
