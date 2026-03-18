package resume

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

// Profile holds the parsed information from a resume.
type Profile struct {
	Skills          []string
	YearsExperience int
	Titles          []string
}

var knownSkills = []string{
	"go", "golang", "python", "java", "javascript", "typescript", "rust", "c++", "c#",
	"kubernetes", "k8s", "docker", "helm", "terraform", "ansible", "puppet", "chef",
	"aws", "gcp", "azure", "cloud", "linux", "unix",
	"prometheus", "grafana", "datadog", "splunk", "elk",
	"grpc", "protobuf", "rest", "graphql",
	"postgresql", "mysql", "redis", "mongodb", "kafka", "rabbitmq",
	"microservices", "distributed systems", "infrastructure", "platform",
	"ci/cd", "github actions", "jenkins", "argocd", "gitops",
}

// yearPattern matches patterns like "2019 – 2022" or "Jan 2020 - Present"
var yearPattern = regexp.MustCompile(`\b(20\d{2})\b`)

// ParseResume downloads (or reads from cache) a PDF resume and extracts a Profile.
func ParseResume(url, cachePath string, cacheTTLDays int) (*Profile, error) {
	pdfPath, err := ensureCached(url, cachePath, cacheTTLDays)
	if err != nil {
		return nil, fmt.Errorf("caching resume: %w", err)
	}

	text, err := extractPDFText(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("extracting PDF text: %w", err)
	}

	return parseText(text), nil
}

// ensureCached returns the local PDF path, downloading if stale/missing.
func ensureCached(url, cachePath string, ttlDays int) (string, error) {
	if cachePath == "" {
		cachePath = filepath.Join(os.TempDir(), "jobscout_resume.pdf")
	}

	info, err := os.Stat(cachePath)
	if err == nil && time.Since(info.ModTime()) < time.Duration(ttlDays)*24*time.Hour {
		slog.Info("using cached resume", "path", cachePath)
		return cachePath, nil
	}

	slog.Info("downloading resume", "url", url)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return "", err
	}

	f, err := os.Create(cachePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}

	return cachePath, nil
}

// extractPDFText reads all plain text from a PDF file.
func extractPDFText(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	plain, err := r.GetPlainText()
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(plain); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// parseText extracts skills, titles, and years of experience from raw text.
func parseText(text string) *Profile {
	lower := strings.ToLower(text)
	profile := &Profile{}

	// Skills: scan for known keywords
	seen := make(map[string]bool)
	for _, skill := range knownSkills {
		if strings.Contains(lower, skill) && !seen[skill] {
			profile.Skills = append(profile.Skills, skill)
			seen[skill] = true
		}
	}

	// Years of experience: find earliest and latest year mentioned
	years := yearPattern.FindAllString(text, -1)
	if len(years) >= 2 {
		earliest, latest := 9999, 0
		for _, y := range years {
			n, _ := strconv.Atoi(y)
			if n < earliest {
				earliest = n
			}
			if n > latest {
				latest = n
			}
		}
		if latest > earliest {
			profile.YearsExperience = latest - earliest
		}
	}

	// Title heuristics: look for common role keywords near the top
	titleKeywords := []string{
		"software engineer", "infrastructure engineer", "platform engineer",
		"sre", "site reliability", "backend engineer", "devops engineer",
		"systems engineer", "software developer",
	}
	for _, t := range titleKeywords {
		if strings.Contains(lower, t) {
			profile.Titles = append(profile.Titles, t)
		}
	}

	slog.Info("parsed resume", "skills", len(profile.Skills), "years", profile.YearsExperience, "titles", profile.Titles)
	return profile
}
