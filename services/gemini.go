package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini api error (status: %d): %s", resp.StatusCode, string(b))
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

Your task is to transform a user's work experience into powerful, impact-focused statements that showcase what they accomplished and the measurable results.

Job Description:
%s

User's Original Experience:
%s

Transform this experience by focusing on IMPACT and ACTIONS. Each statement should follow this formula:
[ACTION VERB] + [WHAT YOU DID] + [HOW/WHY] + [MEASURABLE IMPACT/RESULT]

Please rewrite the experience to:
1. Start each line with a powerful action verb showing what YOU specifically did
2. Include specific technologies, tools, or methodologies used
3. Show measurable impact with numbers, percentages, timeframes, or scale
4. Demonstrate business value (cost savings, revenue increase, efficiency gains, user growth)
5. Highlight team leadership or cross-functional collaboration with specifics
6. Match keywords from the job description where relevant
7. Show problem-solving and initiative

CRITICAL - Use these types of action verbs:
- BUILDING: "Architected", "Built", "Developed", "Engineered", "Created", "Designed"
- IMPROVING: "Optimized", "Reduced", "Increased", "Enhanced", "Streamlined", "Accelerated"
- LEADING: "Led", "Directed", "Managed", "Coordinated", "Spearheaded", "Mentored"
- ANALYZING: "Analyzed", "Identified", "Researched", "Evaluated", "Assessed"
- DELIVERING: "Delivered", "Launched", "Deployed", "Implemented", "Released"

INCLUDE these types of metrics:
- Performance improvements (X% faster, X% more efficient)
- Cost/revenue impact ($X saved, $X generated, X% cost reduction)
- Scale (X users, X transactions, X data processed)
- Time savings (reduced from X to Y, X hours saved per week)
- Team size (led team of X, collaborated with X departments)
- Success rates (increased success rate from X% to Y%)

AVOID these weak phrases:
- "Responsible for", "Worked on", "Helped with", "Assisted in", "Participated in"
- "Various tasks", "Different projects", "Multiple initiatives"
- Generic statements without specific outcomes

IMPORTANT: Return ONLY the optimized description text with each achievement on a separate line. Do NOT include:
- Job title, company name, dates
- Bullet point symbols (• or -)
- Explanations or additional text

Format example:
Architected and deployed a real-time data processing pipeline using Apache Kafka and Python, handling 50M+ daily events and reducing data latency from 2 hours to under 5 minutes

Led cross-functional team of 8 engineers to migrate legacy monolith to microservices architecture, resulting in 60% improvement in deployment frequency and 40% reduction in production incidents

Implemented automated testing framework that increased code coverage from 45% to 95%, reducing critical bugs in production by 80% and saving 20 hours per week in manual testing

Each achievement should demonstrate clear ownership, specific actions, and measurable business impact.`, jobDescription, userExperience)
}

func BuildEducationOptimizationPrompt(education string) string {
	return fmt.Sprintf(`You are an expert resume writer and career coach.

Your task is to optimize a user's education section to make it more professional and impactful.

User's Original Education:
%s

Please optimize the education description by:
1. Making it more professional and concise
2. Highlighting relevant coursework and achievements
3. Using clear, professional language
4. Maintaining the same level of detail but making it more impactful
5. Focusing on relevant academic achievements and skills

IMPORTANT: Return ONLY the optimized education text. Do NOT include:
- Explanations or additional text
- Bullet point symbols (• or -)
- Any header information

Format the response as clean, professional text that can be directly used as the education description.`, education)
}

func BuildSummaryOptimizationPrompt(experience, education string, skills []string) string {
	skillsText := ""
	if len(skills) > 0 {
		skillsText = strings.Join(skills, ", ")
	}

	return fmt.Sprintf(`You are an expert resume writer and career coach.

Your task is to create a compelling professional summary based on the user's experience, education, and skills.

Experience: %s
Education: %s
Skills: %s

Please create a professional summary that:
1. Leads with specific job title/seniority level (e.g., "Senior Software Engineer", "Marketing Manager", "Data Scientist")
2. States years of experience and core technical/functional expertise
3. Highlights specific achievements with metrics where possible
4. Emphasizes relevant technologies, tools, or methodologies
5. Shows concrete impact and accomplishments
6. Uses active, specific language - NO generic buzzwords

CRITICAL - AVOID these overused buzzwords and phrases:
- "Results-oriented"
- "Detail-oriented" 
- "Self-motivated"
- "Team player"
- "Hard-working"
- "Passionate"
- "Dynamic"
- "Proven track record"
- "Extensive experience"
- "Strong background"

INSTEAD, be specific about:
- Exact technologies/tools used
- Quantifiable achievements
- Specific technical skills
- Concrete accomplishments
- Industry expertise
- Leadership experience with team sizes
- Revenue/cost/efficiency impacts

IMPORTANT: Return ONLY the professional summary text. Do NOT include:
- Explanations or additional text
- Bullet point symbols (• or -)
- Any header information
- Generic personality traits

Format the response as a clean, professional summary that immediately demonstrates value through specific expertise and accomplishments.`, experience, education, skillsText)
}

// Grammar improvement prompts
func BuildExperienceGrammarPrompt(experience string) string {
	return fmt.Sprintf(`You are an expert resume writer and editor.

Your task is to improve the grammar, clarity, and professional tone of this work experience description while focusing on IMPACT and ACTIONS.

Original Experience Description:
%s

Transform this experience using this formula: [ACTION VERB] + [WHAT YOU DID] + [HOW/WHY] + [MEASURABLE IMPACT/RESULT]

Please improve this text by:
1. Correcting any grammatical errors
2. Improving sentence structure and flow
3. Using powerful action verbs that show ownership and initiative
4. Adding specific metrics and measurable outcomes where possible
5. Demonstrating business value and concrete results
6. Ensuring consistent tense and voice (past tense for completed roles)
7. Removing redundancy and vague language
8. Keeping the same meaning but enhancing impact

CRITICAL - Use these types of action verbs:
- BUILDING: "Architected", "Built", "Developed", "Engineered", "Created", "Designed"
- IMPROVING: "Optimized", "Reduced", "Increased", "Enhanced", "Streamlined", "Accelerated"
- LEADING: "Led", "Directed", "Managed", "Coordinated", "Spearheaded", "Mentored"
- ANALYZING: "Analyzed", "Identified", "Researched", "Evaluated", "Assessed"
- DELIVERING: "Delivered", "Launched", "Deployed", "Implemented", "Released"

ENHANCE with these types of metrics:
- Performance improvements (X% faster, X% more efficient)
- Cost/revenue impact ($X saved, $X generated, X% cost reduction)
- Scale (X users, X transactions, X data processed)
- Time savings (reduced from X to Y, X hours saved per week)
- Team size (led team of X, collaborated with X departments)
- Success rates (increased success rate from X% to Y%)

AVOID these weak phrases:
- "Responsible for", "Worked on", "Helped with", "Assisted in", "Participated in"
- "Various tasks", "Different projects", "Multiple initiatives"
- Generic statements without specific outcomes

IMPORTANT: Return ONLY the improved description text. Do NOT include:
- Job title, company name, dates
- Any header information
- Explanations or additional text
- Bullet point symbols (• or -)

Format the response as clean text with each achievement on a new line, like this:
Architected and deployed real-time data processing pipeline using Apache Kafka, handling 50M+ daily events and reducing data latency from 2 hours to under 5 minutes
Led cross-functional team of 8 engineers to migrate legacy systems, resulting in 60% improvement in deployment frequency and 40% reduction in production incidents
Implemented automated testing framework that increased code coverage from 45% to 95%, reducing critical bugs by 80% and saving 20 hours per week

Each achievement should demonstrate clear ownership, specific actions, and measurable business impact.`, experience)
}

func BuildSummaryGrammarPrompt(summary string) string {
	return fmt.Sprintf(`You are an expert resume writer and editor.

Your task is to improve the grammar, clarity, and professional tone of this professional summary.

Original Summary:
%s

Please improve this text by:
1. Correcting any grammatical errors
2. Improving sentence structure and flow
3. Using stronger, more professional language
4. Making the tone more compelling and confident
5. Ensuring consistent tense and voice
6. Removing redundancy and improving clarity
7. Eliminating generic buzzwords and replacing with specific details
8. Keeping the same meaning and content level

CRITICAL - REMOVE these overused buzzwords if present:
- "Results-oriented"
- "Detail-oriented" 
- "Self-motivated"
- "Team player"
- "Hard-working"
- "Passionate"
- "Dynamic"
- "Proven track record"
- "Extensive experience"
- "Strong background"

REPLACE buzzwords with:
- Specific job titles and seniority levels
- Concrete technical skills and tools
- Quantifiable achievements and metrics
- Actual accomplishments and impacts
- Industry-specific expertise

IMPORTANT: Return ONLY the improved summary text. Do NOT include:
- Explanations or additional text
- Bullet point symbols (• or -)
- Any header information
- Generic personality traits

Format the response as a clean, professional summary that immediately demonstrates value through specific expertise.`, summary)
}

// Resume advice prompt
func BuildResumeAdvicePrompt(resumeData map[string]interface{}, jobDescription string) string {
	// Convert resume data to readable format
	resumeText := ""
	
	if name, ok := resumeData["name"].(string); ok && name != "" {
		resumeText += fmt.Sprintf("Name: %s\n", name)
	}
	if email, ok := resumeData["email"].(string); ok && email != "" {
		resumeText += fmt.Sprintf("Email: %s\n", email)
	}
	if phone, ok := resumeData["phone"].(string); ok && phone != "" {
		resumeText += fmt.Sprintf("Phone: %s\n", phone)
	}
	
	if summary, ok := resumeData["summary"].(string); ok && summary != "" {
		resumeText += fmt.Sprintf("\nSummary: %s\n", summary)
	}
	
	if experiences, ok := resumeData["experiences"].([]interface{}); ok {
		resumeText += "\nExperience:\n"
		for i, exp := range experiences {
			if expMap, ok := exp.(map[string]interface{}); ok {
				resumeText += fmt.Sprintf("Experience %d:\n", i+1)
				if jobTitle, ok := expMap["jobTitle"].(string); ok && jobTitle != "" {
					resumeText += fmt.Sprintf("  Job Title: %s\n", jobTitle)
				}
				if company, ok := expMap["company"].(string); ok && company != "" {
					resumeText += fmt.Sprintf("  Company: %s\n", company)
				}
				if description, ok := expMap["description"].(string); ok && description != "" {
					resumeText += fmt.Sprintf("  Description: %s\n", description)
				}
			}
		}
	}
	
	if education, ok := resumeData["education"].([]interface{}); ok {
		resumeText += "\nEducation:\n"
		for i, edu := range education {
			if eduMap, ok := edu.(map[string]interface{}); ok {
				resumeText += fmt.Sprintf("Education %d:\n", i+1)
				if degree, ok := eduMap["degree"].(string); ok && degree != "" {
					resumeText += fmt.Sprintf("  Degree: %s\n", degree)
				}
				if school, ok := eduMap["school"].(string); ok && school != "" {
					resumeText += fmt.Sprintf("  School: %s\n", school)
				}
			}
		}
	}
	
	if skills, ok := resumeData["skills"].(string); ok && skills != "" {
		resumeText += fmt.Sprintf("\nSkills: %s\n", skills)
	}

	jobContext := ""
	if jobDescription != "" {
		jobContext = fmt.Sprintf(`

Target Job Description:
%s

Please provide advice specifically tailored to this job posting.`, jobDescription)
	}

	return fmt.Sprintf(`You are an expert resume consultant and career coach with 15+ years of experience.

Analyze this resume and provide specific, actionable advice to improve it:

RESUME:
%s%s

Please provide comprehensive advice covering:

1. **Overall Impression**: What are the resume's strengths and main weaknesses?

2. **Content Quality**: 
   - Are achievements quantified with specific metrics?
   - Are action verbs strong and varied?
   - Is the content relevant and impactful?

3. **Structure & Format**:
   - Is the information well-organized?
   - Are sections properly ordered?
   - Is the content scannable by recruiters?

4. **Missing Elements**: What important information is missing?

5. **Specific Improvements**: Provide 3-5 concrete actions to strengthen this resume

6. **Industry Alignment**: How well does this align with current industry expectations?

IMPORTANT: Provide specific, actionable feedback. Use bullet points for clarity. Be constructive but honest about areas needing improvement.

Format your response with clear sections and specific recommendations.`, resumeText, jobContext)
}

// Cover letter prompt
func BuildCoverLetterPrompt(resumeData map[string]interface{}, jobDescription, companyName string) string {
	// Extract key information from resume
	name := ""
	if n, ok := resumeData["name"].(string); ok {
		name = n
	}
	
	experience := ""
	if experiences, ok := resumeData["experiences"].([]interface{}); ok && len(experiences) > 0 {
		if expMap, ok := experiences[0].(map[string]interface{}); ok {
			if jobTitle, ok := expMap["jobTitle"].(string); ok {
				experience = jobTitle
			}
			if company, ok := expMap["company"].(string); ok && company != "" {
				if experience != "" {
					experience += " at " + company
				} else {
					experience = company
				}
			}
		}
	}
	
	skills := ""
	if s, ok := resumeData["skills"].(string); ok {
		skills = s
	}
	
	summary := ""
	if s, ok := resumeData["summary"].(string); ok {
		summary = s
	}

	companyContext := ""
	if companyName != "" {
		companyContext = fmt.Sprintf("The company name is %s. ", companyName)
	}

	jobContext := ""
	if jobDescription != "" {
		jobContext = fmt.Sprintf(`

Job Description:
%s

Please tailor the cover letter specifically to this role and its requirements.`, jobDescription)
	}

	return fmt.Sprintf(`You are an expert career coach and professional writer.

Create a compelling cover letter for this candidate:

Name: %s
Current/Recent Experience: %s
Key Skills: %s
Professional Summary: %s

%s%s

Please write a professional cover letter that:

1. **Opening**: Strong hook that grabs attention
2. **Value Proposition**: Clearly shows how the candidate's experience matches the role
3. **Specific Examples**: Uses concrete achievements from their background
4. **Company Connection**: Shows genuine interest in the company/role
5. **Call to Action**: Professional closing that encourages next steps

**Requirements:**
- Keep it to 3-4 paragraphs
- Use professional but engaging tone
- Include specific achievements and metrics where possible
- Customize based on the job requirements
- Make it genuine and authentic
- Include proper business letter formatting

**Format:**
[Date]

Dear Hiring Manager,

[Cover letter content]

Sincerely,
%s

IMPORTANT: Return the complete cover letter ready to send. Make it compelling and specific to this opportunity.`, name, experience, skills, summary, companyContext, jobContext, name)
}
