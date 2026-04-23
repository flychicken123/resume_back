package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
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

var (
	langChainOnce  sync.Once
	langChainErr   error
	langChainModel llms.Model

	// Flash model for faster, simpler tasks
	langChainFlashOnce  sync.Once
	langChainFlashErr   error
	langChainFlashModel llms.Model
)

func getLangChainModel() (llms.Model, error) {
	langChainOnce.Do(func() {
		apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
		if apiKey == "" {
			langChainErr = fmt.Errorf("GEMINI_API_KEY environment variable is not set")
			return
		}

		opts := []googleai.Option{
			googleai.WithAPIKey(apiKey),
			googleai.WithDefaultModel("gemini-2.5-flash"),
		}

		llm, err := googleai.New(context.Background(), opts...)
		if err != nil {
			langChainErr = fmt.Errorf("failed to initialize LangChain GoogleAI client: %w", err)
			return
		}

		langChainModel = llm
	})

	return langChainModel, langChainErr
}

func CallGeminiWithAPIKey(prompt string) (string, error) {
	if err := geminiGate.acquire(context.Background()); err != nil {
		return "", err
	}
	defer geminiGate.release()
	llm, err := getLangChainModel()
	if err != nil {
		return "", err
	}

	return llms.GenerateFromSinglePrompt(context.Background(), llm, prompt)
}

// getLangChainFlashModel returns a Gemini Flash model optimized for speed.
func getLangChainFlashModel() (llms.Model, error) {
	langChainFlashOnce.Do(func() {
		apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
		if apiKey == "" {
			langChainFlashErr = fmt.Errorf("GEMINI_API_KEY environment variable is not set")
			return
		}

		opts := []googleai.Option{
			googleai.WithAPIKey(apiKey),
			googleai.WithDefaultModel("gemini-2.5-flash"),
		}

		llm, err := googleai.New(context.Background(), opts...)
		if err != nil {
			langChainFlashErr = fmt.Errorf("failed to initialize LangChain GoogleAI Flash client: %w", err)
			return
		}

		langChainFlashModel = llm
	})

	return langChainFlashModel, langChainFlashErr
}

// CallGeminiFlash calls Gemini Flash model which is 5-10x faster than Pro.
func CallGeminiFlash(prompt string) (string, error) {
	if err := geminiGate.acquire(context.Background()); err != nil {
		return "", err
	}
	defer geminiGate.release()
	llm, err := getLangChainFlashModel()
	if err != nil {
		return "", err
	}

	return llms.GenerateFromSinglePrompt(context.Background(), llm, prompt)
}

// CallGeminiFlashWithTemperature calls Gemini Flash with a specific temperature.
func CallGeminiFlashWithTemperature(prompt string, temperature float64) (string, error) {
	if err := geminiGate.acquire(context.Background()); err != nil {
		return "", err
	}
	defer geminiGate.release()
	llm, err := getLangChainFlashModel()
	if err != nil {
		return "", err
	}
	return llms.GenerateFromSinglePrompt(context.Background(), llm, prompt,
		llms.WithTemperature(temperature))
}

// ClassifyWriteIntent uses a cheap Flash call to determine if a user message
// requests a data mutation that should trigger a tool call.
func ClassifyWriteIntent(userMessage string) bool {
	prompt := fmt.Sprintf(`Classify this user message as ACTION or QUESTION.
ACTION = user wants to change, update, move, create, delete, track, or modify data
  Examples: "move X to rejected", "track my application at Google", "update my skills", "delete that entry"
QUESTION = user is asking something, making conversation, expressing emotion, or requesting information
  Examples: "what is my status?", "I got rejected from Google", "how many jobs did I apply to?", "hello"

Message: "%s"

Reply with one word only: ACTION or QUESTION`, userMessage)

	result, err := CallWithRateLimitRetry(context.Background(), DefaultRetryConfig(),
		func(ctx context.Context) (string, error) {
			return CallGeminiFlashWithTemperature(prompt, 0.0)
		},
		RetryOpts{Endpoint: "classify_intent"},
	)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToUpper(strings.TrimSpace(result)), "ACTION")
}

// CallGeminiStreaming streams tokens via callback and returns the full text.
func CallGeminiStreaming(prompt string, onChunk func(chunk string)) (string, error) {
	if err := geminiGate.acquire(context.Background()); err != nil {
		return "", err
	}
	defer geminiGate.release()
	llm, err := getLangChainModel()
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	_, err = llms.GenerateFromSinglePrompt(context.Background(), llm, prompt,
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			s := string(chunk)
			buf.WriteString(s)
			onChunk(s)
			return nil
		}),
	)
	return buf.String(), err
}

// CallGeminiStreamingWithTemperature streams tokens with temperature control.
func CallGeminiStreamingWithTemperature(prompt string, temperature float64, onChunk func(chunk string)) (string, error) {
	if err := geminiGate.acquire(context.Background()); err != nil {
		return "", err
	}
	defer geminiGate.release()
	llm, err := getLangChainModel()
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	_, err = llms.GenerateFromSinglePrompt(context.Background(), llm, prompt,
		llms.WithTemperature(temperature),
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			s := string(chunk)
			buf.WriteString(s)
			onChunk(s)
			return nil
		}),
	)
	return buf.String(), err
}

// CallGeminiWithTemperature calls Gemini with a specific temperature setting.
// Lower temperature (0.0-0.3) = more focused, consistent, factual output
// Higher temperature (0.7-1.0) = more creative, varied output
// For resume optimization, use low temperature (0.2-0.3) to reduce hallucinations.
func CallGeminiWithTemperature(prompt string, temperature float64) (string, error) {
	if err := geminiGate.acquire(context.Background()); err != nil {
		return "", err
	}
	defer geminiGate.release()
	llm, err := getLangChainModel()
	if err != nil {
		return "", err
	}

	return llms.GenerateFromSinglePrompt(
		context.Background(),
		llm,
		prompt,
		llms.WithTemperature(temperature),
	)
}

// CallGeminiStrict calls Gemini with very low temperature (0.1) for factual tasks
// where we want to minimize hallucination and maximize adherence to source material.
// Use this for resume optimization, polishing, and any task where inventing content is bad.
func CallGeminiStrict(prompt string) (string, error) {
	return CallGeminiWithTemperature(prompt, 0.1)
}

// ValidateOutputWithAI performs a two-pass validation by asking the AI to check
// if the optimized output contains any invented content not in the original.
// Returns the validated output and any detected fabrications.
func ValidateOutputWithAI(original, optimized string) (string, []string, error) {
	validationPrompt := fmt.Sprintf(`You are a strict fact-checker. Compare the ORIGINAL text with the OPTIMIZED text and identify ANY fabricated content.

ORIGINAL TEXT:
%s

OPTIMIZED TEXT:
%s

Check for these specific fabrications:
1. Numbers/percentages not in original (e.g., "50%%" improvement when no percentage existed)
2. Dollar amounts or revenue figures not in original
3. Technologies or tools not mentioned in original
4. Company names or project names not in original
5. Specific metrics (users, transactions, time savings) not in original
6. Job titles or responsibilities not in original

Return ONLY valid JSON:
{
  "has_fabrications": true/false,
  "fabrications": ["list of specific fabricated items found"],
  "clean_version": "If fabrications found, rewrite the optimized text removing ALL fabricated content while keeping improvements to grammar and structure. If no fabrications, return the optimized text unchanged."
}

Be STRICT. If ANY metric, number, or specific detail appears in the optimized version that was not in the original, it is a fabrication.`, original, optimized)

	result, err := CallGeminiStrict(validationPrompt)
	if err != nil {
		// If validation fails, return original optimized text with warning
		return optimized, []string{"validation_failed"}, err
	}

	// Parse the response
	clean := sanitizeLLMJSON(result)

	var response struct {
		HasFabrications bool     `json:"has_fabrications"`
		Fabrications    []string `json:"fabrications"`
		CleanVersion    string   `json:"clean_version"`
	}

	if err := json.Unmarshal([]byte(clean), &response); err != nil {
		// If parsing fails, return original with warning
		return optimized, []string{"parse_failed"}, nil
	}

	if response.HasFabrications && response.CleanVersion != "" {
		return response.CleanVersion, response.Fabrications, nil
	}

	return optimized, response.Fabrications, nil
}

// OptimizeWithValidation performs optimization with low temperature and two-pass validation.
// This is the recommended function for all resume content optimization.
func OptimizeWithValidation(original, prompt string) (string, []string, error) {
	// Step 1: Generate optimized content with low temperature
	optimized, err := CallGeminiStrict(prompt)
	if err != nil {
		return "", nil, err
	}

	// Step 2: Validate the output for fabrications
	validated, fabrications, err := ValidateOutputWithAI(original, optimized)
	if err != nil {
		// Log but continue with the optimized version
		fmt.Printf("Validation error (continuing with optimized): %v\n", err)
		return optimized, fabrications, nil
	}

	if len(fabrications) > 0 {
		fmt.Printf("Fabrications detected and removed: %v\n", fabrications)
	}

	return validated, fabrications, nil
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

	case "executive-serif":
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
- Performance improvements (X%% faster, X%% more efficient)
- Cost/revenue impact ($X saved, $X generated, X%% cost reduction)
- Scale (X users, X transactions, X data processed)
- Time savings (reduced from X to Y, X hours saved per week)
- Team size (led team of X, collaborated with X departments)
- Success rates (increased success rate from X%% to Y%%)

INTEGRITY REQUIREMENTS:
- Do NOT invent new accomplishments, responsibilities, technologies, or metrics that are not supported by the user's original experience.
- Only rephrase, reorganize, or quantify information that already exists in the original description.
- If the original text lacks metrics, keep the improvement qualitative rather than fabricating numbers.

AVOID these weak phrases:
- "Responsible for", "Worked on", "Helped with", "Assisted in", "Participated in"
- "Various tasks", "Different projects", "Multiple initiatives"
- Generic statements without specific outcomes

IMPORTANT: Return ONLY the optimized description text with each achievement on a separate line. Do NOT include:
- Job title, company name, dates
- Bullet point symbols (• or -)
- Explanations, instructions, or additional text
- Example bullets — ONLY rewrite the user's ACTUAL experience

CRITICAL: Output the SAME NUMBER of bullet points as the user's original. Do NOT add extra bullets. Do NOT repeat bullets. Do NOT copy example text. ONLY rephrase what the user actually did.`, jobDescription, userExperience)
}

func BuildEducationOptimizationPrompt(education string, existingEducation ...string) string {
	existingContext := ""
	var mode string
	if len(existingEducation) > 0 && strings.TrimSpace(existingEducation[0]) != "" {
		existingContext = fmt.Sprintf(`
EXISTING EDUCATION (Preserve key details and enhance):
%s
`, strings.TrimSpace(existingEducation[0]))
		mode = "enhance the existing education entry while preserving its key details"
	} else {
		mode = "optimize the education section"
	}

	return fmt.Sprintf(`You are an expert resume writer and career coach.
%s
Your task is to %s to make it more professional and impactful.

User's Original/New Education:
%s

Please optimize the education description by:
1. Making it more professional and concise
2. Highlighting relevant coursework and achievements
3. Using clear, professional language
4. Maintaining the same level of detail but making it more impactful
5. Focusing on relevant academic achievements and skills
6. If existing education is provided, PRESERVE the degree, institution, and key achievements while enhancing presentation

IMPORTANT: Return ONLY the optimized education text. Do NOT include:
- Explanations or additional text
- Bullet point symbols (• or -)
- Any header information

Format the response as clean, professional text that can be directly used as the education description.`, existingContext, mode, education)
}

func BuildSummaryOptimizationPrompt(experience, education string, skills []string, existingSummary ...string) string {
	skillsText := ""
	if len(skills) > 0 {
		skillsText = strings.Join(skills, ", ")
	}

	existingContext := ""
	var mode string
	if len(existingSummary) > 0 && strings.TrimSpace(existingSummary[0]) != "" {
		existingContext = fmt.Sprintf(`
EXISTING SUMMARY (Preserve key points and enhance):
%s
`, strings.TrimSpace(existingSummary[0]))
		mode = "enhance the existing summary while preserving its core message"
	} else {
		mode = "create a compelling professional summary"
	}

	return fmt.Sprintf(`You are an expert resume writer and career coach.
%s
Your task is to %s based on the user's experience, education, and skills.

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
  7. Stays fully aligned with the provided experience, education, and skills without inventing new roles, achievements, or metrics

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

INTEGRITY REQUIREMENTS:
- If an existing summary is provided, PRESERVE its core message and key achievements
- Enhance and polish the existing content rather than replacing it entirely
- Base every statement solely on the supplied experience, education, skills, and existing summary
- Do NOT introduce employers, titles, responsibilities, or outcomes that cannot be inferred from the provided information
- If metrics are absent, keep accomplishments qualitative instead of fabricating numbers

  IMPORTANT: Return ONLY the professional summary text. Do NOT include:
- Explanations or additional text
- Bullet point symbols (• or -)
- Any header information
- Generic personality traits

Format the response as a clean, professional summary that immediately demonstrates value through specific expertise and accomplishments.`, existingContext, mode, experience, education, skillsText)
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
  - Performance improvements (X%% faster, X%% more efficient)
  - Cost/revenue impact ($X saved, $X generated, X%% cost reduction)
  - Scale (X users, X transactions, X data processed)
  - Time savings (reduced from X to Y, X hours saved per week)
  - Team size (led team of X, collaborated with X departments)
  - Success rates (increased success rate from X%% to Y%%)

  INTEGRITY REQUIREMENTS:
  - Preserve the factual content of the original description—do NOT invent new duties, technologies, companies, or outcomes.
  - Only add metrics when they are implied by the original text; otherwise keep statements qualitative without fabricating numbers.

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
Led cross-functional team of 8 engineers to migrate legacy systems, resulting in 60%% improvement in deployment frequency and 40%% reduction in production incidents
Implemented automated testing framework that increased code coverage from 45%% to 95%%, reducing critical bugs by 80%% and saving 20 hours per week

Each achievement should demonstrate clear ownership, specific actions, and measurable business impact.`, experience)
}

func BuildProjectOptimizationPrompt(jobDescription string, projectData string, existingProject ...string) string {
	existingContext := ""
	var mode string
	if len(existingProject) > 0 && strings.TrimSpace(existingProject[0]) != "" {
		existingContext = fmt.Sprintf(`
EXISTING PROJECT (Preserve key details and enhance):
%s
`, strings.TrimSpace(existingProject[0]))
		mode = "enhance the existing project description while preserving its key details and technologies"
	} else {
		mode = "optimize the project description"
	}

	return fmt.Sprintf(`You are an expert resume writer who specializes in optimizing project descriptions for job applications.
%s
Your task is to %s to align with the specific job requirements.

Job Description:
%s

Current/New Project Description:
%s

Please optimize this project by:
1. Highlighting technologies and skills mentioned in the job description
2. Emphasizing relevant accomplishments that match job requirements
3. Using industry-specific keywords from the job posting
4. Quantifying impact where possible
5. Aligning the tone with the company culture suggested by the job posting
6. Focusing on aspects most relevant to the target role
7. Using action verbs that demonstrate the competencies required
8. If existing project is provided, PRESERVE the project name, core technologies, and key achievements while enhancing presentation

Keep the project authentic and truthful while emphasizing the most relevant aspects for this role. Do NOT invent new achievements, technologies, or metrics—only elevate what is already present in the original project description. If specific numbers are missing, keep the statement qualitative rather than fabricating data.

Return ONLY the improved project description text, with each achievement on a new line.
Do NOT include project name, dates, or any header information.`, existingContext, mode, jobDescription, projectData)
}

func BuildProjectGrammarPrompt(projectData string, existingProject ...string) string {
	existingContext := ""
	if len(existingProject) > 0 && strings.TrimSpace(existingProject[0]) != "" {
		existingContext = fmt.Sprintf(`
EXISTING PROJECT (Preserve and enhance):
%s
`, strings.TrimSpace(existingProject[0]))
	}
	return fmt.Sprintf(`You are an expert resume writer and editor specializing in technical project descriptions.
%s
Your task is to improve the grammar, clarity, and professional tone of this project description.

Original/New Project Description:
%s

Transform this project description using this formula: [ACTION VERB] + [WHAT YOU BUILT/DID] + [TECHNOLOGIES USED] + [IMPACT/OUTCOME]

Please improve this text by:
1. Correcting any grammatical errors
2. Improving sentence structure and flow
3. Using powerful technical action verbs (Built, Developed, Architected, Implemented, Designed, Created, Deployed)
4. Highlighting technical skills and tools effectively
5. Adding specific metrics and outcomes where possible (users, performance, scale)
  6. Demonstrating technical complexity and problem-solving
  7. Ensuring consistent tense (past tense for completed projects)
  8. Making it clear what YOU specifically did (not team accomplishments)

  INTEGRITY REQUIREMENTS:
  - If existing project is provided, PRESERVE its key details, technologies, and achievements
  - Preserve the factual details supplied in the original project; do NOT invent new scope, technologies, or outcomes
  - Only add metrics when they are implied by the original description—otherwise keep statements qualitative without fabricating numbers

  ENHANCE with these types of metrics:
- Technical scale (X users, X requests/second, X GB data)
- Performance improvements (X%% faster, X ms latency)
- Code quality metrics (X%% test coverage, X%% reduction in bugs)
- User impact (X downloads, X active users, X rating)
- Technical complexity (X microservices, X APIs, X integrations)

AVOID these weak phrases:
- "Worked on", "Helped with", "Participated in", "Assisted with"
- "Simple", "Basic", "Easy"
- Vague descriptions without technical details

IMPORTANT: Return ONLY the improved description text. Do NOT include:
- Project name or dates
- Any header information
- Explanations or additional text
- Bullet point symbols (• or -)

Format the response as clean text with each achievement on a new line, focusing on technical accomplishments and measurable outcomes.`, existingContext, projectData)
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

// JobReRankCandidate captures the minimal view of a job needed for LLM
// re-ranking of job matches.
type JobReRankCandidate struct {
	Index       int
	JobID       int64
	Title       string
	Company     string
	Location    string
	Department  string
	RemoteType  string
	Description string
	BaseScore   float64
}

// BuildJobReRankingPrompt builds a prompt asking the LLM to assign a relevance
// score (0–10) to each of the provided job candidates for the given resume
// context. The intent is to use these scores as a soft re-ranker on top of the
// existing heuristic scoring.
func BuildJobReRankingPrompt(input ResumeJobMatchInput, jobs []JobReRankCandidate) string {
	var resumeBuilder strings.Builder

	if strings.TrimSpace(input.Position) != "" {
		resumeBuilder.WriteString("Target Position: ")
		resumeBuilder.WriteString(strings.TrimSpace(input.Position))
		resumeBuilder.WriteString("\n")
	}
	if strings.TrimSpace(input.Summary) != "" {
		resumeBuilder.WriteString("Summary: ")
		resumeBuilder.WriteString(strings.TrimSpace(input.Summary))
		resumeBuilder.WriteString("\n")
	}
	if strings.TrimSpace(input.Experience) != "" {
		resumeBuilder.WriteString("\nExperience:\n")
		resumeBuilder.WriteString(strings.TrimSpace(input.Experience))
		resumeBuilder.WriteString("\n")
	}
	if strings.TrimSpace(input.Education) != "" {
		resumeBuilder.WriteString("\nEducation:\n")
		resumeBuilder.WriteString(strings.TrimSpace(input.Education))
		resumeBuilder.WriteString("\n")
	}
	if len(input.Skills) > 0 {
		resumeBuilder.WriteString("\nSkills: ")
		resumeBuilder.WriteString(strings.Join(input.Skills, ", "))
		resumeBuilder.WriteString("\n")
	}
	if strings.TrimSpace(input.PreferredLocation) != "" {
		resumeBuilder.WriteString("\nPreferred Location: ")
		resumeBuilder.WriteString(strings.TrimSpace(input.PreferredLocation))
		resumeBuilder.WriteString("\n")
	}
	if strings.TrimSpace(input.JobDescription) != "" {
		resumeBuilder.WriteString("\nTarget Job Description (if provided by user):\n")
		resumeBuilder.WriteString(strings.TrimSpace(input.JobDescription))
		resumeBuilder.WriteString("\n")
	}
	if input.CandidateYOE > 0 {
		resumeBuilder.WriteString(fmt.Sprintf("\nYears of Experience: %.1f years\n", input.CandidateYOE))
	}

	resumeBlock := resumeBuilder.String()

	var jobsBuilder strings.Builder
	for _, job := range jobs {
		jobsBuilder.WriteString(fmt.Sprintf("Job %d:\n", job.Index))
		if strings.TrimSpace(job.Title) != "" {
			jobsBuilder.WriteString("  Title: ")
			jobsBuilder.WriteString(strings.TrimSpace(job.Title))
			jobsBuilder.WriteString("\n")
		}
		if strings.TrimSpace(job.Company) != "" {
			jobsBuilder.WriteString("  Company: ")
			jobsBuilder.WriteString(strings.TrimSpace(job.Company))
			jobsBuilder.WriteString("\n")
		}
		if strings.TrimSpace(job.Location) != "" {
			jobsBuilder.WriteString("  Location: ")
			jobsBuilder.WriteString(strings.TrimSpace(job.Location))
			jobsBuilder.WriteString("\n")
		}
		if strings.TrimSpace(job.Department) != "" {
			jobsBuilder.WriteString("  Department: ")
			jobsBuilder.WriteString(strings.TrimSpace(job.Department))
			jobsBuilder.WriteString("\n")
		}
		if strings.TrimSpace(job.RemoteType) != "" {
			jobsBuilder.WriteString("  Remote Type: ")
			jobsBuilder.WriteString(strings.TrimSpace(job.RemoteType))
			jobsBuilder.WriteString("\n")
		}
		if job.BaseScore > 0 {
			jobsBuilder.WriteString(fmt.Sprintf("  Baseline Match Score: %.2f\n", job.BaseScore))
		}
		if trimmed := strings.TrimSpace(job.Description); trimmed != "" {
			jobsBuilder.WriteString("  Description: ")
			jobsBuilder.WriteString(trimmed)
			jobsBuilder.WriteString("\n")
		}
		jobsBuilder.WriteString("\n")
	}

	return fmt.Sprintf(`You are an expert career coach and job matching assistant.

The system has already computed baseline match scores between a candidate's resume and several job postings.
Your task is to look at the resume context and the list of jobs, then assign an additional
"llm_score" between 0 and 10 (higher is better) for each job, reflecting how strong the fit is.

CRITICAL RULE - INDUSTRY AND CAREER CATEGORY MATCHING:
First, identify the candidate's industry/career category from their resume (e.g., IT/Software, Healthcare, Manufacturing, Finance, Marketing, etc.).
Then, for EACH job, check if it belongs to the same or closely related industry.

- If a job is in a COMPLETELY DIFFERENT industry or career category than the candidate's background, you MUST assign a score of 0.
- For example: If the candidate has an IT/Software background (developer, engineer, analyst), jobs like "Cold Spray Technician", "Welder", "Nurse", "Truck Driver", "Manufacturing Operator" should receive a score of 0.
- Only jobs that match the candidate's industry/career category should receive scores above 0.

YEARS OF EXPERIENCE (YOE) ALIGNMENT:
If the candidate's years of experience is provided, consider YOE alignment when scoring:
- Jobs requiring similar or less YOE than the candidate has: Score higher (good fit)
- Jobs requiring slightly more YOE (1-2 years more): Still acceptable, moderate score
- Jobs requiring significantly more YOE (3-5 years more): Lower score (stretch role)
- Jobs requiring much more YOE (5+ years more): Very low score (likely under-qualified)

Scoring criteria for jobs that match the industry:
- Clear overlap between the candidate's skills/experience and the job's responsibilities and stack (weight heavily)
- Alignment between seniority level and scope of work
- Years of experience alignment with job requirements
- Location and remote preferences when they are mentioned
- Overall coherence between the resume and job description

--- RESUME CONTEXT ---
%s

--- JOB CANDIDATES ---
%s

Return ONLY valid JSON using this exact shape. Include an entry for every job index shown above:
{
  "scores": [
    { "index": 1, "llm_score": 8.7 },
    { "index": 2, "llm_score": 0, "reason": "Different industry" }
  ]
}

- "index" MUST match the Job N index from the list above.
- "llm_score" MUST be a number between 0 and 10.
- "reason" is optional but encouraged when score is 0 to explain the mismatch.
- Do not include any other fields or text outside this JSON.`, resumeBlock, jobsBuilder.String())
}

// ParseJobReRankingScores interprets the LLM JSON response for job re-ranking
// and returns a map from job index (1-based) to an LLM score in the range
// [0, 10]. If parsing fails, it returns an empty map.
func ParseJobReRankingScores(raw string, jobCount int) map[int]float64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[int]float64{}
	}

	// Strip common markdown fences if present.
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	type item struct {
		Index    int     `json:"index"`
		LLMScore float64 `json:"llm_score"`
	}
	var wrapper struct {
		Scores []item `json:"scores"`
	}
	if err := json.Unmarshal([]byte(trimmed), &wrapper); err != nil {
		return map[int]float64{}
	}

	out := make(map[int]float64, len(wrapper.Scores))
	for _, s := range wrapper.Scores {
		if s.Index <= 0 || s.Index > jobCount {
			continue
		}
		score := s.LLMScore
		if score < 0 {
			score = 0
		}
		if score > 10 {
			score = 10
		}
		out[s.Index] = score
	}
	return out
}

// BuildJobFitExplanationPrompt builds a prompt that asks the LLM to explain
// why a particular job match is a good fit for the current resume.
func BuildJobFitExplanationPrompt(resumeData map[string]interface{}, match map[string]interface{}) string {
	var sb strings.Builder

	name, _ := resumeData["name"].(string)
	email, _ := resumeData["email"].(string)
	phone, _ := resumeData["phone"].(string)
	summary, _ := resumeData["summary"].(string)

	if name != "" {
		sb.WriteString("Name: ")
		sb.WriteString(name)
		sb.WriteString("\n")
	}
	if email != "" {
		sb.WriteString("Email: ")
		sb.WriteString(email)
		sb.WriteString("\n")
	}
	if phone != "" {
		sb.WriteString("Phone: ")
		sb.WriteString(phone)
		sb.WriteString("\n")
	}
	if summary != "" {
		sb.WriteString("\nSummary: ")
		sb.WriteString(summary)
		sb.WriteString("\n")
	}

	if experiences, ok := resumeData["experiences"].([]interface{}); ok && len(experiences) > 0 {
		sb.WriteString("\nExperience:\n")
		for i, exp := range experiences {
			expMap, ok := exp.(map[string]interface{})
			if !ok {
				continue
			}
			sb.WriteString(fmt.Sprintf("Experience %d:\n", i+1))
			if v, ok := expMap["jobTitle"].(string); ok && strings.TrimSpace(v) != "" {
				sb.WriteString("  Job Title: ")
				sb.WriteString(strings.TrimSpace(v))
				sb.WriteString("\n")
			}
			if v, ok := expMap["company"].(string); ok && strings.TrimSpace(v) != "" {
				sb.WriteString("  Company: ")
				sb.WriteString(strings.TrimSpace(v))
				sb.WriteString("\n")
			}
			if v, ok := expMap["description"].(string); ok && strings.TrimSpace(v) != "" {
				sb.WriteString("  Description: ")
				sb.WriteString(strings.TrimSpace(v))
				sb.WriteString("\n")
			}
		}
	}

	if skills, ok := resumeData["skills"].(string); ok && strings.TrimSpace(skills) != "" {
		sb.WriteString("\nSkills: ")
		sb.WriteString(strings.TrimSpace(skills))
		sb.WriteString("\n")
	}

	resumeBlock := sb.String()

	jobTitle, _ := match["job_title"].(string)
	companyName, _ := match["company_name"].(string)
	jobLocation, _ := match["job_location"].(string)
	jobDepartment, _ := match["job_department"].(string)
	jobRemoteType, _ := match["job_remote_type"].(string)
	employmentType, _ := match["job_employment_type"].(string)
	jobDescription, _ := match["job_description"].(string)

	// Extract skill gap data — prefer LLM-extracted skills when available
	var matchedSkills, missingSkills, requiredSkills []string

	// Use extracted_skills (LLM-generated) if available, otherwise fall back to required_skills (regex)
	if es, ok := match["extracted_skills"].([]interface{}); ok && len(es) > 0 {
		for _, s := range es {
			if str, ok := s.(string); ok {
				requiredSkills = append(requiredSkills, str)
			}
		}
		// Recompute matched/missing from extracted_skills vs user skills
		userSkillSet := make(map[string]bool)
		if userSkills, ok := resumeData["skills"].(string); ok {
			for _, s := range strings.Split(strings.ToLower(userSkills), ",") {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					userSkillSet[trimmed] = true
				}
			}
		}
		for _, req := range requiredSkills {
			if userSkillSet[strings.ToLower(req)] {
				matchedSkills = append(matchedSkills, req)
			} else {
				missingSkills = append(missingSkills, req)
			}
		}
	} else {
		// Fall back to pre-computed skill gaps
		if ms, ok := match["matched_skills"].([]interface{}); ok {
			for _, s := range ms {
				if str, ok := s.(string); ok {
					matchedSkills = append(matchedSkills, str)
				}
			}
		}
		if ms, ok := match["missing_skills"].([]interface{}); ok {
			for _, s := range ms {
				if str, ok := s.(string); ok {
					missingSkills = append(missingSkills, str)
				}
			}
		}
		if rs, ok := match["required_skills"].([]interface{}); ok {
			for _, s := range rs {
				if str, ok := s.(string); ok {
					requiredSkills = append(requiredSkills, str)
				}
			}
		}
	}

	var jobBuilder strings.Builder
	if jobTitle != "" {
		jobBuilder.WriteString("Job Title: ")
		jobBuilder.WriteString(jobTitle)
		jobBuilder.WriteString("\n")
	}
	if companyName != "" {
		jobBuilder.WriteString("Company: ")
		jobBuilder.WriteString(companyName)
		jobBuilder.WriteString("\n")
	}
	if jobLocation != "" {
		jobBuilder.WriteString("Location: ")
		jobBuilder.WriteString(jobLocation)
		jobBuilder.WriteString("\n")
	}
	if jobDepartment != "" {
		jobBuilder.WriteString("Department: ")
		jobBuilder.WriteString(jobDepartment)
		jobBuilder.WriteString("\n")
	}
	if jobRemoteType != "" {
		jobBuilder.WriteString("Remote Type: ")
		jobBuilder.WriteString(jobRemoteType)
		jobBuilder.WriteString("\n")
	}
	if employmentType != "" {
		jobBuilder.WriteString("Employment Type: ")
		jobBuilder.WriteString(employmentType)
		jobBuilder.WriteString("\n")
	}
	if trimmed := strings.TrimSpace(jobDescription); trimmed != "" {
		jobBuilder.WriteString("\nJob Description:\n")
		jobBuilder.WriteString(trimmed)
		jobBuilder.WriteString("\n")
	}

	// Add match score and seniority context
	if score, ok := match["match_score"].(float64); ok && score > 0 {
		jobBuilder.WriteString(fmt.Sprintf("\nMatch Score: %.1f/100\n", score))
	}
	if seniority, ok := match["seniority"].(string); ok && seniority != "" {
		jobBuilder.WriteString("Job Seniority: ")
		jobBuilder.WriteString(seniority)
		jobBuilder.WriteString("\n")
	}

	jobBlock := jobBuilder.String()

	// Build skill analysis section
	var skillAnalysis string
	if len(requiredSkills) > 0 {
		skillFitPercent := 0
		if len(requiredSkills) > 0 {
			skillFitPercent = (len(matchedSkills) * 100) / len(requiredSkills)
		}
		skillAnalysis = fmt.Sprintf(`
--- SKILL ANALYSIS ---
Required Skills: %s
Candidate's Matching Skills: %s
Skills Gap (candidate lacks): %s
Skill Fit: %d%% (%d of %d skills matched)
`,
			strings.Join(requiredSkills, ", "),
			strings.Join(matchedSkills, ", "),
			strings.Join(missingSkills, ", "),
			skillFitPercent, len(matchedSkills), len(requiredSkills))
	}

	return fmt.Sprintf(`You are an expert career coach who explains job matches to candidates in clear, concrete, actionable language.

Below is the candidate's resume context, a job match, and skill gap analysis.

--- RESUME CONTEXT ---
%s

--- JOB MATCH ---
%s%s

Write 3 to 5 concise, specific reasons explaining this job fit. Structure your response as:

1. SKILL ALIGNMENT (required): Reference specific matching skills from the analysis. Example: "Your Python and AWS expertise directly matches their core stack requirements."

2. EXPERIENCE FIT (required): Connect their work history to the role. Example: "Leading backend systems at [Company] prepared you for this distributed architecture role."

3. GROWTH OPPORTUNITY (if skill gaps exist): Frame missing skills positively. Example: "This role offers hands-on Kubernetes experience to round out your DevOps toolkit."

4. LOCATION/CULTURE FIT (if relevant): Comment on remote/location alignment.

5. ACTIONABLE TIP (optional): One specific suggestion to strengthen their application. Example: "Emphasize your microservices experience from [Company] in your cover letter."

Guidelines:
- Be SPECIFIC - reference actual skills, companies, and technologies by name
- Be HONEST about skill gaps but frame them as growth opportunities
- Avoid generic phrases like "great fit" or "well-qualified"
- Each reason: 10-20 words, confident tone, candidate-facing
- Do not mention match scores or AI
- If match score > 80: use confident language ("strong alignment", "directly relevant")
- If match score 60-80: acknowledge fit with areas to grow
- If match score < 60: focus on transferable skills and growth potential
- If job seniority differs from candidate's level: address it (e.g., "stretch into leadership" or "well within your expertise")

Return ONLY valid JSON:
{
  "reasons": [
    "Reason 1 (skill alignment)...",
    "Reason 2 (experience fit)...",
    "Reason 3 (growth/tip)..."
  ]
}`, resumeBlock, jobBlock, skillAnalysis)
}

// ParseFitReasons extracts fit reason strings from an LLM response.
func ParseFitReasons(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	var wrapper struct {
		Reasons []string `json:"reasons"`
	}
	if err := json.Unmarshal([]byte(trimmed), &wrapper); err == nil && len(wrapper.Reasons) > 0 {
		out := make([]string, 0, len(wrapper.Reasons))
		for _, r := range wrapper.Reasons {
			if r = strings.TrimSpace(r); r != "" {
				out = append(out, r)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	// Fallback: plain text lines
	var reasons []string
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip numbered prefixes like "1. " or "- "
		if len(line) > 2 && (line[0] >= '0' && line[0] <= '9') && (line[1] == '.' || line[1] == ')') {
			line = strings.TrimSpace(line[2:])
		}
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "• ")
		line = strings.TrimSpace(line)
		if line != "" {
			reasons = append(reasons, line)
		}
	}
	return reasons
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
	// Fallback: experience field may be a plain string from the DB
	if experience == "" {
		if expStr, ok := resumeData["experience"].(string); ok && expStr != "" {
			experience = expStr
		}
	}

	skills := ""
	if s, ok := resumeData["skills"].(string); ok {
		skills = s
	}
	// Fallback: skills may be stored as skillsCategorized
	if skills == "" {
		if s, ok := resumeData["skillsCategorized"].(string); ok && s != "" {
			skills = s
		}
	}

	summary := ""
	if s, ok := resumeData["summary"].(string); ok {
		summary = s
	}

	education := ""
	if e, ok := resumeData["education"].(string); ok {
		education = e
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

	return fmt.Sprintf(`You are an expert career coach and professional writer. You MUST output ONLY the final cover letter text. Do NOT ask questions, request more information, provide templates, or include any commentary. Use whatever information is provided and write the best cover letter possible with it.

Write a compelling cover letter for this candidate:

Name: %s
Current/Recent Experience: %s
Key Skills: %s
Professional Summary: %s
Education: %s

%s%s

The cover letter must:

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
- Do NOT use placeholder text in square brackets like "[Project 1 Name]" or "[Briefly describe the project and its goals]"
- If you don't know specific project names or details, describe them naturally (for example, "one of my recent infrastructure projects") instead of leaving blanks
- Do NOT ask follow-up questions or request additional information
- Do NOT include example fill-ins, templates, or instructions
- Output ONLY the cover letter itself, nothing else

**Format:**
[Today's Date]

Dear Hiring Manager,

[Cover letter content - 3 to 4 paragraphs]

Sincerely,
%s

IMPORTANT: Return ONLY the complete cover letter ready to send. Do NOT include any preamble, commentary, or follow-up questions.`, name, experience, skills, summary, education, companyContext, jobContext, name)
}

// BuildRecommendationLetterPrompt builds a prompt for generating a third-party
// recommendation letter (reference letter) about the candidate instead of a
// first-person cover letter written by the candidate.
func BuildRecommendationLetterPrompt(resumeData map[string]interface{}, jobDescription, companyName, position string) string {
	// Reuse the same extraction logic as the cover letter prompt.
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

	targetRole := strings.TrimSpace(position)
	if targetRole == "" && jobDescription != "" {
		// Best-effort: if the combined job description contains a "Position:" line,
		// pull it out for the prompt.
		lines := strings.Split(jobDescription, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Position:") {
				targetRole = strings.TrimSpace(strings.TrimPrefix(line, "Position:"))
				break
			}
		}
	}

	companyContext := ""
	if companyName != "" {
		companyContext = fmt.Sprintf("The target company is %s. ", companyName)
	}

	jobContext := ""
	if jobDescription != "" {
		jobContext = fmt.Sprintf(`

Job Description:
%s

When appropriate, align the recommendation to this role and its requirements.`, jobDescription)
	}

	roleSnippet := ""
	if targetRole != "" {
		roleSnippet = fmt.Sprintf(" for the %s position", targetRole)
	}

	return fmt.Sprintf(`You are an expert hiring manager and professional reference letter writer.

Create a compelling third-party recommendation letter for this candidate%[1]s:

Candidate Name: %[2]s
Current/Recent Experience: %[3]s
Key Skills: %[4]s
Professional Summary: %[5]s

%[6]s%[7]s

Write the letter FROM THE PERSPECTIVE of a credible former manager or senior colleague who has worked closely with the candidate.

Requirements:
- Use a professional, supportive tone.
- Describe how you know the candidate and for how long.
- Highlight 2–3 concrete projects, impacts, or achievements with specific details or metrics where possible.
- Emphasize strengths, work ethic, collaboration, and problem-solving.
- If job description context is provided, connect the candidate's strengths to that role.
- Close with a strong endorsement and offer to provide further information.
- Do NOT use placeholder text in square brackets like "[Project 1 Name]" or "[Briefly describe the project and its goals]".
- If you don't know specific project names or details, describe them naturally (for example, "a complex data platform migration project") instead of leaving blanks.

Formatting:
- Use standard business letter formatting and paragraphs.
- Do NOT write in the first person as the candidate; always write as the recommender.

Return the complete recommendation letter ready to send.`, roleSnippet, name, experience, skills, summary, companyContext, jobContext)
}

// FetchAndProcessURL fetches content from a URL and processes it with AI
func FetchAndProcessURL(url string, prompt string) (string, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Set user agent to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	// Fetch the URL
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch URL: status code %d", resp.StatusCode)
	}

	// Read the response body (limit to 1MB to avoid huge pages)
	limitedReader := io.LimitReader(resp.Body, 1024*1024)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	htmlContent := string(bodyBytes)

	// Process with AI to extract job description
	fullPrompt := fmt.Sprintf(`Given this HTML content from a web page, %s

HTML Content:
%s`, prompt, htmlContent)

	// Call Gemini to process the content
	result, err := CallGeminiWithAPIKey(fullPrompt)
	if err != nil {
		return "", fmt.Errorf("failed to process content with AI: %v", err)
	}

	return result, nil
}

// BuildExperienceOptimizationPromptWithSkills builds a prompt that includes skill gap context
// for more targeted optimization while strictly maintaining integrity.
func BuildExperienceOptimizationPromptWithSkills(jobDescription, userExperience string, matchedSkills, missingSkills []string) string {
	var skillContext string
	if len(matchedSkills) > 0 || len(missingSkills) > 0 {
		skillContext = "\n\nSKILL CONTEXT:\n"
		if len(matchedSkills) > 0 {
			skillContext += fmt.Sprintf("- Skills you HAVE that match this job: %s\n", strings.Join(matchedSkills, ", "))
			skillContext += "  → EMPHASIZE these skills where they genuinely appear in the experience\n"
		}
		if len(missingSkills) > 0 {
			skillContext += fmt.Sprintf("- Skills the job wants that you're MISSING: %s\n", strings.Join(missingSkills, ", "))
			skillContext += "  → Do NOT add these skills. Instead, highlight transferable skills that demonstrate learning ability\n"
		}
	}

	return fmt.Sprintf(`You are an expert resume writer. Your task is to enhance this work experience description to better align with the job requirements.

CRITICAL INTEGRITY RULES (NEVER VIOLATE):
1. NEVER invent accomplishments, metrics, technologies, or responsibilities not present in the original
2. NEVER fabricate numbers, percentages, or dollar amounts
3. NEVER add skills or technologies the candidate didn't mention
4. If the original lacks metrics, keep statements qualitative - do NOT make up numbers
5. Preserve the candidate's voice and authentic experience
6. Only rephrase, reorganize, and highlight - do NOT create new content

Job Description:
%s
%s
User's Original Experience:
%s

ENHANCEMENT GUIDELINES:
1. Start each statement with a strong action verb (Led, Built, Developed, Implemented)
2. Highlight skills from the MATCHED list where they genuinely appear
3. Use keywords from the job description where they naturally fit the existing experience
4. Improve clarity and professional tone while keeping the same meaning
5. Remove weak phrases like "Responsible for", "Helped with", "Worked on"

IMPORTANT: Return ONLY the enhanced description text. Each achievement on a new line. No bullet points, headers, or explanations.`, jobDescription, skillContext, userExperience)
}

// BuildProjectOptimizationPromptWithSkills builds a prompt that includes skill gap context
// for project optimization while maintaining strict integrity.
func BuildProjectOptimizationPromptWithSkills(jobDescription, projectData, existingProject string, matchedSkills, missingSkills []string) string {
	existingContext := ""
	if strings.TrimSpace(existingProject) != "" {
		existingContext = fmt.Sprintf("\nEXISTING PROJECT (preserve key details):\n%s\n", strings.TrimSpace(existingProject))
	}

	var skillContext string
	if len(matchedSkills) > 0 || len(missingSkills) > 0 {
		skillContext = "\n\nSKILL CONTEXT:\n"
		if len(matchedSkills) > 0 {
			skillContext += fmt.Sprintf("- Your matching skills: %s\n", strings.Join(matchedSkills, ", "))
			skillContext += "  → Emphasize these where they genuinely appear in the project\n"
		}
		if len(missingSkills) > 0 {
			skillContext += fmt.Sprintf("- Skills you're missing: %s\n", strings.Join(missingSkills, ", "))
			skillContext += "  → Do NOT add these. Focus on transferable skills instead\n"
		}
	}

	return fmt.Sprintf(`You are an expert resume writer. Enhance this project description to align with the job requirements.

CRITICAL INTEGRITY RULES (NEVER VIOLATE):
1. NEVER invent features, technologies, metrics, or outcomes not in the original
2. NEVER fabricate user counts, performance numbers, or scale metrics
3. NEVER add technologies the candidate didn't actually use
4. If metrics are missing, describe impact qualitatively - do NOT make up numbers
5. Preserve the project's authentic scope and the candidate's actual contributions
%s
Job Description:
%s
%s
Current Project Description:
%s

ENHANCEMENT GUIDELINES:
1. Use strong technical action verbs (Architected, Engineered, Deployed, Implemented)
2. Highlight technologies from your MATCHED skills where they exist in the project
3. Emphasize aspects most relevant to the target role
4. Improve technical clarity without inventing new scope

IMPORTANT: Return ONLY the enhanced project description. Each achievement on a new line. No headers or explanations.`, existingContext, jobDescription, skillContext, projectData)
}

// BuildSummaryOptimizationPromptWithSkills builds a prompt for summary generation
// with skill context and strict integrity constraints.
func BuildSummaryOptimizationPromptWithSkills(experience, education string, skills []string, existingSummary, jobDescription string, matchedSkills, missingSkills []string) string {
	skillsText := ""
	if len(skills) > 0 {
		skillsText = strings.Join(skills, ", ")
	}

	existingContext := ""
	if strings.TrimSpace(existingSummary) != "" {
		existingContext = fmt.Sprintf("\nEXISTING SUMMARY (preserve core message):\n%s\n", strings.TrimSpace(existingSummary))
	}

	jobContext := ""
	if strings.TrimSpace(jobDescription) != "" {
		jobContext = fmt.Sprintf("\nTARGET JOB:\n%s\n", strings.TrimSpace(jobDescription))
	}

	var skillContext string
	if len(matchedSkills) > 0 || len(missingSkills) > 0 {
		skillContext = "\n\nSKILL ALIGNMENT:\n"
		if len(matchedSkills) > 0 {
			skillContext += fmt.Sprintf("- Your skills that match: %s\n", strings.Join(matchedSkills, ", "))
		}
		if len(missingSkills) > 0 {
			skillContext += fmt.Sprintf("- Skills to develop: %s (mention eagerness to learn, NOT that you have these)\n", strings.Join(missingSkills, ", "))
		}
	}

	return fmt.Sprintf(`You are an expert resume writer. Create a professional summary that positions the candidate for the target role.

CRITICAL INTEGRITY RULES (NEVER VIOLATE):
1. NEVER claim skills, titles, or experience the candidate doesn't have
2. NEVER invent years of experience or fabricate accomplishments
3. NEVER add technologies not mentioned in their experience/skills
4. Base every statement on the provided experience, education, and skills
5. If existing summary is provided, preserve its authentic content
6. For missing skills, express learning interest - do NOT claim proficiency
%s%s
Experience: %s
Education: %s
Skills: %s
%s
SUMMARY GUIDELINES:
1. Lead with actual job title/level based on their experience
2. State real years of experience (infer from experience section, don't invent)
3. Highlight matched skills prominently
4. Use specific technologies they actually know
5. Keep it 2-4 sentences, confident but honest

AVOID: "Results-oriented", "detail-oriented", "passionate", generic buzzwords

IMPORTANT: Return ONLY the professional summary text. No headers or explanations.`, existingContext, jobContext, experience, education, skillsText, skillContext)
}

// ValidateNoHallucinations performs a lightweight check to detect potential hallucinations.
// It compares the optimized output against the original to flag suspicious additions.
// Returns the validated text (potentially with warnings) and any detected issues.
func ValidateNoHallucinations(original, optimized string) (string, []string) {
	var issues []string
	
	originalLower := strings.ToLower(original)
	optimizedLower := strings.ToLower(optimized)
	
	// Check for specific percentage patterns that weren't in original
	percentagePatterns := []string{
		"100%", "95%", "90%", "85%", "80%", "75%", "70%", "60%", "50%", "40%", "30%", "20%",
	}
	for _, pct := range percentagePatterns {
		if strings.Contains(optimizedLower, pct) && !strings.Contains(originalLower, pct) {
			// Check if any percentage was in original
			hasAnyPercent := strings.Contains(originalLower, "%")
			if !hasAnyPercent {
				issues = append(issues, fmt.Sprintf("Added percentage '%s' not in original", pct))
			}
		}
	}
	
	// Check for dollar amounts that weren't in original
	if (strings.Contains(optimizedLower, "$") || strings.Contains(optimizedLower, "million") || 
		strings.Contains(optimizedLower, "billion")) && 
		!strings.Contains(originalLower, "$") && 
		!strings.Contains(originalLower, "million") &&
		!strings.Contains(originalLower, "billion") {
		issues = append(issues, "Added dollar amounts or revenue figures not in original")
	}
	
	// Check for specific large numbers that weren't in original
	largeNumberPatterns := []string{
		"10,000", "50,000", "100,000", "1 million", "10 million", "100 million",
		"10000", "50000", "100000", "1m", "10m", "100m",
	}
	for _, num := range largeNumberPatterns {
		if strings.Contains(optimizedLower, num) && !strings.Contains(originalLower, num) {
			issues = append(issues, fmt.Sprintf("Added large number '%s' not in original", num))
		}
	}
	
	// If issues detected, we still return the text but log the concerns
	// The frontend can decide whether to show warnings to the user
	return optimized, issues
}

// ValidateAndCleanOutput validates the optimized output and removes likely hallucinations.
// This is a more aggressive version that attempts to clean the output.
func ValidateAndCleanOutput(original, optimized string) string {
	validated, issues := ValidateNoHallucinations(original, optimized)

	if len(issues) > 0 {
		// Log the issues for monitoring
		// In production, you might want to use a proper logger
		fmt.Printf("Hallucination detection: %v\n", issues)
	}

	return validated
}

// JobRelevanceCandidate represents a job to be checked for relevance
type JobRelevanceCandidate struct {
	Index      int
	Title      string
	Department string
}

// JobRelevanceFilterResult contains the AI filtering results
type JobRelevanceFilterResult struct {
	RelevantIndices []int             // Indices of jobs that are relevant
	FilteredCount   int               // Number of jobs filtered out
}

// BuildJobRelevanceFilterPrompt builds a prompt for AI to filter jobs by career relevance
func BuildJobRelevanceFilterPrompt(input ResumeJobMatchInput, jobs []JobRelevanceCandidate) string {
	var resumeBuilder strings.Builder

	// Build concise resume summary for the prompt
	if strings.TrimSpace(input.Position) != "" {
		resumeBuilder.WriteString("Target Position: ")
		resumeBuilder.WriteString(strings.TrimSpace(input.Position))
		resumeBuilder.WriteString("\n")
	}
	if strings.TrimSpace(input.Summary) != "" {
		resumeBuilder.WriteString("Summary: ")
		// Truncate summary to first 500 chars to save tokens
		summary := strings.TrimSpace(input.Summary)
		if len(summary) > 500 {
			summary = summary[:500] + "..."
		}
		resumeBuilder.WriteString(summary)
		resumeBuilder.WriteString("\n")
	}
	if len(input.Skills) > 0 {
		resumeBuilder.WriteString("Skills: ")
		resumeBuilder.WriteString(strings.Join(input.Skills, ", "))
		resumeBuilder.WriteString("\n")
	}
	if input.CandidateYOE > 0 {
		resumeBuilder.WriteString(fmt.Sprintf("Years of Experience: %.1f\n", input.CandidateYOE))
	}

	resumeBlock := resumeBuilder.String()

	// Build job list
	var jobsBuilder strings.Builder
	for _, job := range jobs {
		jobsBuilder.WriteString(fmt.Sprintf("%d. %s", job.Index, job.Title))
		if job.Department != "" {
			jobsBuilder.WriteString(fmt.Sprintf(" [%s]", job.Department))
		}
		jobsBuilder.WriteString("\n")
	}

	return fmt.Sprintf(`You are a career matching expert. Your task is to filter out OBVIOUSLY IRRELEVANT jobs from a list while keeping any job that could reasonably match a candidate's background.

CANDIDATE PROFILE:
%s

JOB LISTINGS TO EVALUATE:
%s

INSTRUCTIONS:
1. Identify the candidate's primary career field from their position and skills
2. Keep jobs that are in the same field OR related/adjacent fields
3. ONLY filter out jobs that are clearly in a completely different industry

WHAT TO FILTER OUT (be conservative - only filter these):
- Manufacturing/industrial jobs (welder, machinist, spray technician) for office workers
- Healthcare roles (nurse, medical technician) for non-healthcare candidates
- Trades jobs (electrician, plumber, HVAC) for non-trades candidates
- Driving/logistics jobs (truck driver, delivery) for desk workers
- Food service/hospitality for professional candidates

WHAT TO KEEP (even if not a perfect match):
- Any tech-adjacent role for tech candidates (technical support, solutions engineer, etc.)
- Management roles across departments (engineering managers, project managers)
- Analyst roles across fields (business analyst, data analyst, operations analyst)
- Cross-functional roles (technical writer, developer relations, sales engineer)
- Roles with transferable skills (customer success for support background, etc.)

IMPORTANT: When in doubt, KEEP the job. Let the scoring algorithm handle relevance ranking.
Only filter jobs that are OBVIOUSLY from a completely different career path.

Return JSON with the indices of jobs to KEEP:
{
  "relevant_indices": [1, 2, 3, 4, 6, 7, 8],
  "reasoning": "Kept most jobs. Filtered out Cold Spray Technician (#5) as manufacturing trade unrelated to software."
}`, resumeBlock, jobsBuilder.String())
}

// ParseJobRelevanceFilterResponse parses the AI response for job relevance filtering
func ParseJobRelevanceFilterResponse(raw string, totalJobs int) JobRelevanceFilterResult {
	result := JobRelevanceFilterResult{
		RelevantIndices: make([]int, 0),
		FilteredCount:   0,
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		// If parsing fails, return all jobs as relevant (fail open)
		for i := 1; i <= totalJobs; i++ {
			result.RelevantIndices = append(result.RelevantIndices, i)
		}
		return result
	}

	// Strip markdown fences if present
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	var response struct {
		RelevantIndices []int  `json:"relevant_indices"`
		Reasoning       string `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		// If parsing fails, return all jobs as relevant (fail open)
		for i := 1; i <= totalJobs; i++ {
			result.RelevantIndices = append(result.RelevantIndices, i)
		}
		return result
	}

	// Validate indices are within range
	seen := make(map[int]bool)
	for _, idx := range response.RelevantIndices {
		if idx >= 1 && idx <= totalJobs && !seen[idx] {
			result.RelevantIndices = append(result.RelevantIndices, idx)
			seen[idx] = true
		}
	}

	result.FilteredCount = totalJobs - len(result.RelevantIndices)
	return result
}

// BuildCareerFieldClassificationPrompt builds a prompt to classify candidate's career field
func BuildCareerFieldClassificationPrompt(position string, skills []string, summary string) string {
	var sb strings.Builder

	sb.WriteString("Position: ")
	sb.WriteString(strings.TrimSpace(position))
	sb.WriteString("\n")

	if len(skills) > 0 {
		sb.WriteString("Skills: ")
		sb.WriteString(strings.Join(skills, ", "))
		sb.WriteString("\n")
	}

	if trimmed := strings.TrimSpace(summary); trimmed != "" {
		// Truncate summary to save tokens
		if len(trimmed) > 300 {
			trimmed = trimmed[:300] + "..."
		}
		sb.WriteString("Summary: ")
		sb.WriteString(trimmed)
		sb.WriteString("\n")
	}

	return fmt.Sprintf(`Classify this candidate's primary career field based on their profile.

CANDIDATE PROFILE:
%s

Return ONLY one of these career fields that best matches:
- SOFTWARE_ENGINEERING (developers, engineers, architects, DevOps, QA, SRE)
- DATA_SCIENCE (data scientists, ML engineers, data analysts, AI researchers)
- PRODUCT_MANAGEMENT (product managers, TPMs, program managers)
- DESIGN (UX/UI designers, graphic designers, design leads)
- SALES (account executives, sales reps, BDRs, account managers)
- MARKETING (marketing managers, growth, content, SEO, brand)
- FINANCE (accountants, financial analysts, controllers, FP&A)
- OPERATIONS (operations managers, supply chain, logistics)
- HR_RECRUITING (recruiters, HR managers, people ops)
- CUSTOMER_SUCCESS (CSMs, support engineers, success managers)
- OTHER (if none of the above fit)

Response format (JSON only):
{"career_field": "SOFTWARE_ENGINEERING"}`, sb.String())
}

// ParseCareerFieldClassificationResponse parses the AI response for career field
func ParseCareerFieldClassificationResponse(raw string) CareerField {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return CareerFieldUnknown
	}

	// Strip markdown fences if present
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	var response struct {
		CareerField string `json:"career_field"`
	}

	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		// Try to extract career field from raw text as fallback
		upperRaw := strings.ToUpper(trimmed)
		for _, field := range []CareerField{
			CareerFieldSoftwareEngineering,
			CareerFieldDataScience,
			CareerFieldProductManagement,
			CareerFieldDesign,
			CareerFieldSales,
			CareerFieldMarketing,
			CareerFieldFinance,
			CareerFieldOperations,
			CareerFieldHRRecruiting,
			CareerFieldCustomerSuccess,
		} {
			if strings.Contains(upperRaw, string(field)) {
				return field
			}
		}
		return CareerFieldUnknown
	}

	// Validate and return
	field := CareerField(strings.ToUpper(strings.TrimSpace(response.CareerField)))
	switch field {
	case CareerFieldSoftwareEngineering,
		CareerFieldDataScience,
		CareerFieldProductManagement,
		CareerFieldDesign,
		CareerFieldSales,
		CareerFieldMarketing,
		CareerFieldFinance,
		CareerFieldOperations,
		CareerFieldHRRecruiting,
		CareerFieldCustomerSuccess,
		CareerFieldOther:
		return field
	default:
		return CareerFieldUnknown
	}
}

// BuildJobClassificationPrompt builds a prompt that extracts both career field and required skills from a job posting.
func BuildJobClassificationPrompt(title, description string) string {
	desc := strings.TrimSpace(description)
	if len(desc) > 2000 {
		desc = desc[:2000] + "..."
	}
	return fmt.Sprintf(`Analyze this job posting and return:
1. The career field (one of: SOFTWARE_ENGINEERING, DATA_SCIENCE, PRODUCT_MANAGEMENT, DESIGN, SALES, MARKETING, FINANCE, OPERATIONS, HR_RECRUITING, CUSTOMER_SUCCESS, OTHER)
2. A list of required and preferred technical/professional skills mentioned or implied
3. The seniority level (one of: intern, entry, mid, senior, staff, lead)

JOB TITLE: %s

JOB DESCRIPTION:
%s

Return JSON only:
{"career_field": "SOFTWARE_ENGINEERING", "skills": ["react", "typescript", "aws", "kubernetes"], "seniority": "senior"}

Rules for skills:
- Use lowercase canonical names (e.g. "react" not "React.js")
- Include skills explicitly mentioned AND skills clearly implied (e.g. "distributed systems" implies "kubernetes", "docker", "microservices")
- Focus on required/preferred skills, not nice-to-haves
- Return 5-20 skills max
- Do NOT include soft skills (communication, teamwork)

Rules for seniority:
- intern: internships, co-ops, apprenticeships
- entry: junior, new grad, 0-2 years experience
- mid: mid-level, 2-5 years, no seniority prefix in title
- senior: senior-level, 5+ years
- staff: staff engineer/designer — above senior individual contributor
- lead: lead, principal, manager, director, VP, C-level`, title, desc)
}

// JobClassificationResult holds the parsed result of job classification.
type JobClassificationResult struct {
	CareerField string   `json:"career_field"`
	Skills      []string `json:"skills"`
	Seniority   string   `json:"seniority"`
}

var validSeniorityValues = map[string]bool{
	"intern": true, "entry": true, "mid": true,
	"senior": true, "staff": true, "lead": true,
}

// ParseJobClassificationResponse parses the AI response for job classification.
// Returns career field, skills, and seniority string.
func ParseJobClassificationResponse(raw string) (CareerField, []string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return CareerFieldUnknown, nil, "mid"
	}
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	var result JobClassificationResult
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return CareerFieldUnknown, nil, "mid"
	}

	field := CareerField(strings.ToUpper(strings.TrimSpace(result.CareerField)))
	switch field {
	case CareerFieldSoftwareEngineering, CareerFieldDataScience, CareerFieldProductManagement,
		CareerFieldDesign, CareerFieldSales, CareerFieldMarketing, CareerFieldFinance,
		CareerFieldOperations, CareerFieldHRRecruiting, CareerFieldCustomerSuccess, CareerFieldOther:
		// valid
	default:
		field = CareerFieldUnknown
	}

	seniority := strings.ToLower(strings.TrimSpace(result.Seniority))
	if !validSeniorityValues[seniority] {
		seniority = "mid"
	}

	// Normalize skills to lowercase
	skills := make([]string, 0, len(result.Skills))
	for _, s := range result.Skills {
		if trimS := strings.TrimSpace(strings.ToLower(s)); trimS != "" {
			skills = append(skills, trimS)
		}
	}
	return field, skills, seniority
}
