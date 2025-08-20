package services

import (
	"fmt"
	"log"
	"os"
	"time"
)

type EmailNotificationService struct {
	smtpHost     string
	smtpPort     string
	smtpUsername string
	smtpPassword string
	fromEmail    string
}

type EmailTemplate struct {
	Subject string
	Body    string
}

func NewEmailNotificationService() *EmailNotificationService {
	return &EmailNotificationService{
		smtpHost:     os.Getenv("SMTP_HOST"),
		smtpPort:     os.Getenv("SMTP_PORT"),
		smtpUsername: os.Getenv("SMTP_USERNAME"),
		smtpPassword: os.Getenv("SMTP_PASSWORD"),
		fromEmail:    os.Getenv("FROM_EMAIL"),
	}
}

func (s *EmailNotificationService) SendJobApplicationConfirmation(userEmail, userName, companyName, positionTitle, status string) error {
	log.Printf("Sending job application confirmation email to: %s", userEmail)
	
	var template EmailTemplate
	
	switch status {
	case "submitted":
		template = EmailTemplate{
			Subject: fmt.Sprintf("✅ Job Application Submitted - %s at %s", positionTitle, companyName),
			Body: fmt.Sprintf(`
Hello %s,

Great news! Your job application has been successfully submitted.

Job Details:
• Position: %s
• Company: %s
• Status: Successfully Submitted
• Submitted: %s

What's Next:
The hiring team at %s will review your application and contact you if you're a good fit for the role.

We'll keep you updated on any status changes.

Best regards,
AI Resume Team
			`, userName, positionTitle, companyName, time.Now().Format("January 2, 2006 at 3:04 PM"), companyName),
		}
	case "login_required":
		template = EmailTemplate{
			Subject: fmt.Sprintf("🔐 Action Required - %s at %s", positionTitle, companyName),
			Body: fmt.Sprintf(`
Hello %s,

Your job application for %s at %s requires manual login.

Next Steps:
1. Log in to the job site manually
2. Navigate to the job posting
3. Complete the application process

We've saved your application details for reference.

Best regards,
AI Resume Team
			`, userName, positionTitle, companyName),
		}
	case "external_application":
		template = EmailTemplate{
			Subject: fmt.Sprintf("🔗 External Application Required - %s at %s", positionTitle, companyName),
			Body: fmt.Sprintf(`
Hello %s,

The job posting for %s at %s requires applying directly on the company website.

Next Steps:
1. Visit the company's career page
2. Find the job posting
3. Complete the application process manually

We've saved your application details for reference.

Best regards,
AI Resume Team
			`, userName, positionTitle, companyName),
		}
	default:
		template = EmailTemplate{
			Subject: fmt.Sprintf("📝 Job Application Update - %s at %s", positionTitle, companyName),
			Body: fmt.Sprintf(`
Hello %s,

We have an update on your job application for %s at %s.

Status: %s

Please check your dashboard for more details.

Best regards,
AI Resume Team
			`, userName, positionTitle, companyName, status),
		}
	}
	
	// For now, just log the email instead of actually sending it
	// In production, you would integrate with an email service like SendGrid, AWS SES, etc.
	log.Printf("EMAIL NOTIFICATION:")
	log.Printf("To: %s", userEmail)
	log.Printf("Subject: %s", template.Subject)
	log.Printf("Body: %s", template.Body)
	
	return nil
}

func (s *EmailNotificationService) SendJobApplicationStatusUpdate(userEmail, userName, companyName, positionTitle, oldStatus, newStatus string) error {
	log.Printf("Sending job application status update email to: %s", userEmail)
	
	statusMessages := map[string]string{
		"submitted":             "✅ Successfully submitted to company",
		"login_required":        "🔐 Manual login required",
		"external_application":  "🔗 External application required",
		"automation_failed":     "❌ Automation failed - manual application needed",
		"processing":           "⏳ Currently being processed",
		"under_review":         "👀 Under review by hiring team",
		"interview_scheduled":  "📅 Interview scheduled",
		"rejected":             "❌ Application was not successful",
		"accepted":             "🎉 Congratulations! You got the job",
	}
	
	statusMessage := statusMessages[newStatus]
	if statusMessage == "" {
		statusMessage = newStatus
	}
	
	template := EmailTemplate{
		Subject: fmt.Sprintf("📄 Application Status Update - %s at %s", positionTitle, companyName),
		Body: fmt.Sprintf(`
Hello %s,

Your job application status has been updated.

Job Details:
• Position: %s
• Company: %s
• Previous Status: %s
• New Status: %s

%s

You can view more details in your dashboard.

Best regards,
AI Resume Team
		`, userName, positionTitle, companyName, oldStatus, newStatus, statusMessage),
	}
	
	// Log the email notification
	log.Printf("EMAIL STATUS UPDATE:")
	log.Printf("To: %s", userEmail)
	log.Printf("Subject: %s", template.Subject)
	log.Printf("Body: %s", template.Body)
	
	return nil
}