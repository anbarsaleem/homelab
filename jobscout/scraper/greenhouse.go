package scraper

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

const greenhouseBaseURL = "https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true"

type greenhouseResponse struct {
	Jobs []greenhouseJob `json:"jobs"`
}

type greenhouseJob struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
	AbsoluteURL string `json:"absolute_url"`
}

func scrapeGreenhouse(company Company, client *http.Client) ([]Job, error) {
	url := fmt.Sprintf(greenhouseBaseURL, company.Slug)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "JobScout/1.0 (+https://github.com/anbarsaleem/homelab)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("greenhouse API returned %d for %s", resp.StatusCode, company.Slug)
	}

	var result greenhouseResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	jobs := make([]Job, 0, len(result.Jobs))
	for _, gj := range result.Jobs {
		jobs = append(jobs, Job{
			ID:          strconv.FormatInt(gj.ID, 10),
			Title:       gj.Title,
			Company:     company.Name,
			Location:    gj.Location.Name,
			Description: gj.Content,
			URL:         gj.AbsoluteURL,
		})
	}

	slog.Info("greenhouse scraped", "company", company.Name, "jobs", len(jobs))
	return jobs, nil
}

// scrapeGreenhouseWithDelay wraps scrapeGreenhouse with a polite delay.
func scrapeGreenhouseWithDelay(company Company, client *http.Client, delay time.Duration) ([]Job, error) {
	time.Sleep(delay)
	return scrapeGreenhouse(company, client)
}
