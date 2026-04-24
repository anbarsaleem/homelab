package email

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/smtp"
	"strings"
	"time"

	"github.com/anbarsaleem/homelab/jobscout/scraper"
)

//go:embed template.html
var templateFS embed.FS

// Config holds SMTP configuration.
type Config struct {
	To       string
	From     string
	SMTPHost string
	SMTPPort int
	Password string
}

type templateData struct {
	Date            string
	Jobs            []scraper.Job
	TotalJobs       int
	FeedbackBaseURL string
}

// Send renders the HTML template and delivers it via Gmail SMTP.
func Send(cfg Config, jobs []scraper.Job, feedbackBaseURL string) error {
	tmplBytes, err := templateFS.ReadFile("template.html")
	if err != nil {
		return fmt.Errorf("reading template: %w", err)
	}

	tmpl, err := template.New("email").Parse(string(tmplBytes))
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	data := templateData{
		Date:            time.Now().Format("January 2, 2006"),
		Jobs:            jobs,
		TotalJobs:       len(jobs),
		FeedbackBaseURL: feedbackBaseURL,
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return fmt.Errorf("rendering template: %w", err)
	}

	msg := buildMessage(cfg.From, cfg.To, fmt.Sprintf("JobScout Digest — %s (%d roles)", data.Date, len(jobs)), body.String())

	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	auth := smtp.PlainAuth("", cfg.From, cfg.Password, cfg.SMTPHost)

	if err := smtp.SendMail(addr, auth, cfg.From, []string{cfg.To}, []byte(msg)); err != nil {
		return fmt.Errorf("sending email: %w", err)
	}

	slog.Info("email delivered", "to", cfg.To, "jobs", len(jobs))
	return nil
}

func buildMessage(from, to, subject, htmlBody string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("From: %s\r\n", from))
	sb.WriteString(fmt.Sprintf("To: %s\r\n", to))
	sb.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(htmlBody)
	return sb.String()
}
