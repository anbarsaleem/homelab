package scraper

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const leverBaseURL = "https://api.lever.co/v0/postings/%s?mode=json"

type leverPosting struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	HostedURL  string `json:"hostedUrl"`
	Categories struct {
		Location string `json:"location"`
		Team     string `json:"team"`
	} `json:"categories"`
	DescriptionPlain string `json:"descriptionPlain"`
	Additional       string `json:"additional"`
}

func scrapeLever(company Company, client *http.Client) ([]Job, error) {
	url := fmt.Sprintf(leverBaseURL, company.Slug)

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
		return nil, fmt.Errorf("lever API returned %d for %s", resp.StatusCode, company.Slug)
	}

	var postings []leverPosting
	if err := json.NewDecoder(resp.Body).Decode(&postings); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	jobs := make([]Job, 0, len(postings))
	for _, lp := range postings {
		desc := lp.DescriptionPlain
		if desc == "" {
			desc = lp.Additional
		}
		jobs = append(jobs, Job{
			ID:          lp.ID,
			Title:       lp.Text,
			Company:     company.Name,
			Location:    lp.Categories.Location,
			Description: desc,
			URL:         lp.HostedURL,
		})
	}

	slog.Info("lever scraped", "company", company.Name, "jobs", len(jobs))
	return jobs, nil
}

func scrapeLeverWithDelay(company Company, client *http.Client, delay time.Duration) ([]Job, error) {
	time.Sleep(delay)
	return scrapeLever(company, client)
}
