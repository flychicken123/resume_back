package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
)

type GeminiRequest struct {
	Contents []Content `json:"contents"`
}

type Content struct {
	Parts []Part `json:"parts"`
	Role  string `json:"role"`
}

type Part struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}
type Experience struct {
	Company  string `json:"company"`
	Role     string `json:"role"`
	Duration string `json:"duration"`
	Tasks    string `json:"tasks"`
}

func CallGeminiWithAPIKey(prompt string) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY environment variable is not set")
	}
	
	url := "https://generativelanguage.googleapis.com/v1/models/gemini-1.5-pro:generateContent?key=" + apiKey

	requestBody := GeminiRequest{
		Contents: []Content{
			{
				Role: "user",
				Parts: []Part{
					{Text: prompt},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("Gemini API error (Status: %d): %s", resp.StatusCode, string(b))
	}

	var gemResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gemResp); err != nil {
		return "", err
	}

	if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no predictions returned")
	}

	return gemResp.Candidates[0].Content.Parts[0].Text, nil
}

func BuildResumePrompt(name, email, phone, summary, experience, education string, skills []string, format string) string {
	formatInstructions := getFormatInstructions(format)

	return fmt.Sprintf(`You are an expert resume writer. Create a %s resume using the provided information.

IMPORTANT: Format the resume as a proper .docx document with the following structure:

**RESUME FORMAT: %s**
%s

**CONTENT TO INCLUDE:**

CONTACT INFORMATION:
Name: %s
Email: %s
Phone: %s

PROFESSIONAL SUMMARY:
%s

EXPERIENCE:
%s

EDUCATION:
%s

SKILLS:
%s

**FORMATTING REQUIREMENTS:**
- Use professional fonts (Arial, Calibri, or Times New Roman)
- Use consistent formatting without bullet points
- Include proper spacing between sections
- Use bold for section headers
- Use italics for company names and dates
- Keep line spacing consistent (1.15 or 1.5)
- Use proper margins (1 inch on all sides)

**RESUME STRUCTURE:**
1. Contact Information (Name, Email, Phone)
2. Professional Summary (if provided)
3. Experience (without bullet points)
4. Education (with details)
5. Skills (organized by category if applicable)

Return a properly formatted resume ready for .docx conversion. Use clear, professional language and ensure all information is accurately represented.`, format, format, formatInstructions, name, email, phone, summary, experience, education, strings.Join(skills, ", "))
}

func getFormatInstructions(format string) string {
	switch format {
	case "color-block":
		return `- Use colorful section headers with blue background (#3498db)
- Include visual color blocks for each section
- Modern layout with left border accents
- Use Arial font family
- Include color-coded section titles
- Visual hierarchy with color-coded elements
- Creative and modern design approach
- Professional yet visually appealing`

	case "industry-manager":
		return `- Use sophisticated Georgia font family
- Include thick border lines under section headers
- Professional layout suitable for management roles
- Use executive-level typography
- Include comprehensive section details
- Leadership-focused formatting
- Premium styling for senior positions
- Traditional yet refined appearance`

	case "social-media-marketing":
		return `- Use Calibri font with red accent colors (#e74c3c)
- Include creative visual elements
- Modern card-based layout with shadows
- Marketing-focused design elements
- Use engaging visual hierarchy
- Creative typography and spacing
- Eye-catching design for creative roles
- Professional yet creative appearance`

	default:
		return `- Use clear section headers
- Use clean formatting for experience descriptions without bullet points
- Use action verbs and quantifiable achievements
- Keep formatting clean and professional
- Use consistent spacing and alignment`
	}
}

func BuildExperienceOptimizationPrompt(jobDescription, userExperience string) string {
	return fmt.Sprintf(`You are an expert resume writer and career coach.

Your task is to optimize a user's work experience description to better match a specific job description.

Job Description:
%s

User's Original Experience:
%s

Please optimize the user's experience description by:
1. Using relevant keywords from the job description
2. Highlighting achievements that align with the job requirements
3. Using action verbs and quantifiable results where possible
4. Maintaining the same level of detail but making it more relevant
5. Making the language more professional and impactful

IMPORTANT: Return ONLY the optimized description text without bullet points. Do NOT include:
- Job title
- Company name
- Dates
- Any header information
- Explanations or additional text
- Bullet points (• or -)

Format the response as clean text that can be directly used as the experience description.`, jobDescription, userExperience)
}

func getAccessToken() (string, error) {
	ctx := context.Background()
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", err
	}
	tokenSource := creds.TokenSource
	token, err := tokenSource.Token()
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}
