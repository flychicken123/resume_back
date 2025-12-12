package services

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/smtp"
	"os"
	"strconv"
	"strings"
)

type EmailService struct {
	host     string
	port     int
	username string
	password string
	from     string
	enabled  bool
	useSSL   bool
}

func NewEmailService() *EmailService {
	host := os.Getenv("SMTP_HOST")
	portStr := os.Getenv("SMTP_PORT")
	username := os.Getenv("SMTP_USERNAME")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SUPPORT_EMAIL")
	useSSL := strings.EqualFold(os.Getenv("SMTP_SSL"), "true")

	port := 465 // default to implicit SSL unless overridden
	if parsed, err := strconv.Atoi(portStr); err == nil && parsed > 0 {
		port = parsed
	}

	enabled := host != "" && port != 0 && username != "" && password != "" && from != ""

	return &EmailService{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		enabled:  enabled,
		useSSL:   useSSL,
	}
}

func (s *EmailService) Enabled() bool {
	return s != nil && s.enabled
}

func (s *EmailService) SendSupportEmail(subject, body string) error {
	if !s.Enabled() {
		return fmt.Errorf("email service not configured")
	}
	return s.sendEmail([]string{s.from}, subject, body, false)
}

func (s *EmailService) SendEmail(to string, subject, body string) error {
	if !s.Enabled() {
		return fmt.Errorf("email service not configured")
	}
	recipient := strings.TrimSpace(to)
	if recipient == "" {
		return errors.New("missing recipient")
	}
	return s.sendEmail([]string{recipient}, subject, body, false)
}

// SendHTMLEmail sends an email with HTML content type.
func (s *EmailService) SendHTMLEmail(to string, subject, htmlBody string) error {
	if !s.Enabled() {
		return fmt.Errorf("email service not configured")
	}
	recipient := strings.TrimSpace(to)
	if recipient == "" {
		return errors.New("missing recipient")
	}
	return s.sendEmail([]string{recipient}, subject, htmlBody, true)
}

func (s *EmailService) sendEmail(recipients []string, subject, body string, isHTML bool) error {
	if !s.Enabled() {
		return fmt.Errorf("email service not configured")
	}

	contentType := "text/plain"
	if isHTML {
		contentType = "text/html"
	}

	fromHeader := s.buildFromHeader()
	headers := []string{
		fmt.Sprintf("From: %s", fromHeader),
		fmt.Sprintf("To: %s", strings.Join(recipients, ", ")),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		fmt.Sprintf("Content-Type: %s; charset=\"utf-8\"", contentType),
	}

	msg := []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body + "\r\n")

	if s.useSSL {
		return s.sendMailImplicitTLS(recipients, msg)
	}
	return s.sendMailStartTLS(recipients, msg)
}

func (s *EmailService) sendMailStartTLS(recipients []string, msg []byte) error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	tlsConfig := &tls.Config{
		ServerName: s.host,
	}
	// Upgrade connection to TLS when possible.
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}

	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	if ok, _ := client.Extension("AUTH"); ok {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(s.fromAddress()); err != nil {
		return err
	}
	for _, r := range recipients {
		if err := client.Rcpt(r); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (s *EmailService) sendMailImplicitTLS(recipients []string, msg []byte) error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	tlsConfig := &tls.Config{
		ServerName: s.host,
	}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return err
	}
	defer client.Close()

	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	if ok, _ := client.Extension("AUTH"); ok {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(s.fromAddress()); err != nil {
		return err
	}
	for _, r := range recipients {
		if err := client.Rcpt(r); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func (s *EmailService) buildFromHeader() string {
	from := s.fromAddress()
	display := "HiHired"
	return fmt.Sprintf("%s <%s>", display, from)
}

func (s *EmailService) fromAddress() string {
	from := strings.TrimSpace(s.from)
	if from == "" || !strings.Contains(strings.ToLower(from), "hihired.org") {
		from = "no-reply@hihired.org"
	}
	return from
}
