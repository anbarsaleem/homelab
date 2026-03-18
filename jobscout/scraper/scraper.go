package scraper

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/anbarsaleem/homelab/jobscout/metrics"
)

const requestDelay = 1500 * time.Millisecond

// ScrapeAll iterates over all companies, dispatches to the right API client,
// records Prometheus metrics, and returns all collected jobs.
func ScrapeAll(companies []Company) []Job {
	client := &http.Client{Timeout: 30 * time.Second}
	var all []Job
	first := true

	for _, company := range companies {
		// Skip delay before the very first request.
		delay := requestDelay
		if first {
			delay = 0
			first = false
		}

		start := time.Now()
		var (
			jobs []Job
			err  error
		)

		switch company.Board {
		case BoardGreenhouse:
			jobs, err = scrapeGreenhouseWithDelay(company, client, delay)
		case BoardLever:
			jobs, err = scrapeLeverWithDelay(company, client, delay)
		default:
			slog.Warn("unknown board type, skipping", "company", company.Name, "board", company.Board)
			continue
		}

		duration := time.Since(start).Seconds()
		metrics.ScrapeDuration.Observe(duration)

		if err != nil {
			slog.Error("scrape failed", "company", company.Name, "error", err)
			metrics.ScrapeErrors.WithLabelValues(company.Name).Inc()
			continue
		}

		metrics.JobsScraped.WithLabelValues(company.Name).Add(float64(len(jobs)))
		all = append(all, jobs...)
	}

	return all
}
