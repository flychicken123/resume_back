package services

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/smtp"
	"os"
	"strconv"
	"strings"

	"time"

	"resumeai/utils"
)

type EmailService struct {
	host     string
	port     int
	username string
	password string
	from     string
	supportTo string
	enabled  bool
	useSSL   bool
	logger   *utils.Logger
}

func NewEmailService(logger *utils.Logger) *EmailService {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	portStr := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	username := strings.TrimSpace(os.Getenv("SMTP_USERNAME"))
	password := os.Getenv("SMTP_PASSWORD")
	supportEmail := strings.TrimSpace(os.Getenv("SUPPORT_EMAIL"))
	from := strings.TrimSpace(os.Getenv("EMAIL_FROM"))
	if from == "" {
		from = username
	}
	if from == "" {
		from = supportEmail
	}
	supportTo := strings.TrimSpace(os.Getenv("SUPPORT_EMAIL_TO"))
	if supportTo == "" {
		supportTo = supportEmail
	}

	port := 465 // default to implicit SSL unless overridden
	if parsed, err := strconv.Atoi(portStr); err == nil && parsed > 0 {
		port = parsed
	}

	sslEnv := strings.TrimSpace(os.Getenv("SMTP_SSL"))
	useSSL := strings.EqualFold(sslEnv, "true")
	if sslEnv == "" && port == 465 {
		useSSL = true
	}

	enabled := host != "" && port != 0 && username != "" && password != "" && from != "" && supportTo != ""
	if logger == nil {
		logger = utils.NewLogger()
	}

	return &EmailService{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		supportTo: supportTo,
		enabled:  enabled,
		useSSL:   useSSL,
		logger:   logger,
	}
}

func (s *EmailService) Enabled() bool {
	return s != nil && s.enabled
}

func (s *EmailService) SendSupportEmail(subject, body string) error {
	if !s.Enabled() {
		return fmt.Errorf("email service not configured")
	}
	recipient := strings.TrimSpace(s.supportTo)
	if recipient == "" {
		return errors.New("missing support recipient")
	}
	return s.sendEmail([]string{recipient}, subject, body, false, "support")
}

func (s *EmailService) SendEmail(to string, subject, body string) error {
	if !s.Enabled() {
		return fmt.Errorf("email service not configured")
	}
	recipient := strings.TrimSpace(to)
	if recipient == "" {
		return errors.New("missing recipient")
	}
	return s.sendEmail([]string{recipient}, subject, body, false, "plain")
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
	return s.sendEmail([]string{recipient}, subject, htmlBody, true, "html")
}

func (s *EmailService) sendEmail(recipients []string, subject, body string, isHTML bool, kind string) error {
	if !s.Enabled() {
		return fmt.Errorf("email service not configured")
	}
	cleaned, err := sanitizeRecipients(recipients)
	if err != nil {
		return err
	}
	recipients = cleaned

	start := time.Now()
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

	logData := s.baseLogData(recipients, subject, isHTML, kind)
	s.logger.Debug("email send starting", logData)

	transport := "starttls"
	if s.useSSL {
		transport = "tls"
	}

	if s.useSSL {
		if err := s.sendMailImplicitTLS(recipients, msg); err != nil {
			logData["transport"] = transport
			logData["elapsed_ms"] = time.Since(start).Milliseconds()
			s.logger.Warn("email send failed", map[string]interface{}{"error": err.Error(), "details": logData})
			return err
		}
		logData["transport"] = transport
		logData["elapsed_ms"] = time.Since(start).Milliseconds()
		s.logger.Info("email sent", logData)
		return nil
	}
	if err := s.sendMailStartTLS(recipients, msg); err != nil {
		logData["transport"] = transport
		logData["elapsed_ms"] = time.Since(start).Milliseconds()
		s.logger.Warn("email send failed", map[string]interface{}{"error": err.Error(), "details": logData})
		return err
	}
	logData["transport"] = transport
	logData["elapsed_ms"] = time.Since(start).Milliseconds()
	s.logger.Info("email sent", logData)
	return nil
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
	if from == "" {
		from = strings.TrimSpace(s.username)
	}
	if from == "" || !strings.Contains(from, "@") {
		from = "no-reply@hihired.org"
	}
	return from
}

func (s *EmailService) baseLogData(recipients []string, subject string, isHTML bool, kind string) map[string]interface{} {
	data := map[string]interface{}{
		"kind":        kind,
		"is_html":     isHTML,
		"to_count":    len(recipients),
		"to_domains":  recipientDomains(recipients),
		"subject_len": len(subject),
		"smtp_host":   s.host,
		"smtp_port":   s.port,
		"from":        s.fromAddress(),
	}

	if strings.EqualFold(os.Getenv("EMAIL_LOG_SUBJECT"), "true") {
		data["subject"] = subject
	}
	if strings.EqualFold(os.Getenv("EMAIL_LOG_RECIPIENTS"), "true") {
		data["to"] = recipients
	} else {
		data["to"] = maskRecipients(recipients)
	}
	return data
}

func sanitizeRecipients(recipients []string) ([]string, error) {
	cleaned := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		value := strings.TrimSpace(recipient)
		if value == "" {
			continue
		}
		cleaned = append(cleaned, value)
	}
	if len(cleaned) == 0 {
		return nil, errors.New("missing recipients")
	}
	return cleaned, nil
}

func recipientDomains(recipients []string) []string {
	seen := map[string]struct{}{}
	var domains []string
	for _, r := range recipients {
		_, domain := splitEmail(r)
		if domain == "" {
			continue
		}
		domain = strings.ToLower(domain)
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		domains = append(domains, domain)
	}
	return domains
}

func maskRecipients(recipients []string) []string {
	masked := make([]string, 0, len(recipients))
	for _, r := range recipients {
		local, domain := splitEmail(r)
		if local == "" || domain == "" {
			masked = append(masked, "")
			continue
		}
		first := local[:1]
		masked = append(masked, fmt.Sprintf("%s***@%s", first, domain))
	}
	return masked
}

func splitEmail(addr string) (string, string) {
	trimmed := strings.TrimSpace(addr)
	at := strings.LastIndex(trimmed, "@")
	if at <= 0 || at >= len(trimmed)-1 {
		return "", ""
	}
	return trimmed[:at], trimmed[at+1:]
}
