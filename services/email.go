package services

import (
	"fmt"
	"net/smtp"
	"os"
	"strconv"
)

type EmailService struct {
	host     string
	port     int
	username string
	password string
	from     string
	enabled  bool
}

func NewEmailService() *EmailService {
	host := os.Getenv("SMTP_HOST")
	portStr := os.Getenv("SMTP_PORT")
	username := os.Getenv("SMTP_USERNAME")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SUPPORT_EMAIL")

	port, _ := strconv.Atoi(portStr)
	enabled := host != "" && port != 0 && username != "" && password != "" && from != ""

	return &EmailService{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		enabled:  enabled,
	}
}

func (s *EmailService) Enabled() bool {
	return s != nil && s.enabled
}

func (s *EmailService) SendSupportEmail(subject, body string) error {
	if !s.Enabled() {
		return fmt.Errorf("email service not configured")
	}

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	auth := smtp.PlainAuth("", s.username, s.password, s.host)

	msg := []byte(fmt.Sprintf("Subject: %s\r\n", subject) +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\r\n\r\n" +
		body + "\r\n")

	return smtp.SendMail(addr, auth, s.from, []string{s.from}, msg)
}
