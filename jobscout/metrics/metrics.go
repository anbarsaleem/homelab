package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	JobsScraped = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "jobscout_jobs_scraped_total",
			Help: "Total number of jobs scraped, by company.",
		},
		[]string{"company"},
	)

	JobsMatchedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "jobscout_jobs_matched_total",
			Help: "Total number of jobs that passed matching/ranking.",
		},
	)

	EmailsSentTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "jobscout_emails_sent_total",
			Help: "Total number of digest emails sent.",
		},
	)

	ScrapeErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "jobscout_scrape_errors_total",
			Help: "Total scrape errors, by company.",
		},
		[]string{"company"},
	)

	ScrapeDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "jobscout_scrape_duration_seconds",
			Help:    "Scrape duration per company request.",
			Buckets: prometheus.DefBuckets,
		},
	)
)

// Register all metrics with the default Prometheus registry.
func Register() {
	prometheus.MustRegister(
		JobsScraped,
		JobsMatchedTotal,
		EmailsSentTotal,
		ScrapeErrors,
		ScrapeDuration,
	)
}
