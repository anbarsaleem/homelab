package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/anbarsaleem/homelab/jobscout/email"
	"github.com/anbarsaleem/homelab/jobscout/feedback"
	"github.com/anbarsaleem/homelab/jobscout/learner"
	"github.com/anbarsaleem/homelab/jobscout/matcher"
	"github.com/anbarsaleem/homelab/jobscout/metrics"
	"github.com/anbarsaleem/homelab/jobscout/resume"
	"github.com/anbarsaleem/homelab/jobscout/scraper"
	"github.com/anbarsaleem/homelab/jobscout/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robfig/cron/v3"
)

func main() {
	configPath := flag.String("config", "config.yml", "Path to config file")
	runNow := flag.Bool("run-now", false, "Run job immediately instead of waiting for schedule")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if cfg.GmailAppPassword == "" {
		slog.Warn("GMAIL_APP_PASSWORD not set — emails will not be sent")
	}

	metrics.Register()

	// Open SQLite store.
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		slog.Error("failed to open database", "path", cfg.DBPath, "error", err)
		os.Exit(1)
	}
	defer db.Close()

	job := func() {
		slog.Info("starting job scout run")

		profile, err := resume.ParseResume(cfg.Resume.URL, cfg.Resume.CachePath, cfg.Resume.CacheTTLDays)
		if err != nil {
			slog.Warn("resume parse failed, using fallback", "error", err, "fallback_yoe", cfg.Resume.FallbackYearsExperience)
			profile = &resume.Profile{}
		}
		if profile.YearsExperience == 0 {
			if cfg.Resume.FallbackYearsExperience == 0 {
				slog.Error("years of experience is 0 and no fallback_years_experience set in config — aborting run")
				return
			}
			profile.YearsExperience = cfg.Resume.FallbackYearsExperience
			slog.Info("using fallback YOE", "years", profile.YearsExperience)
		}
		slog.Info("resume profile ready", "skills", len(profile.Skills), "years_experience", profile.YearsExperience)

		companies := make([]scraper.Company, 0, len(cfg.Companies))
		for _, c := range cfg.Companies {
			companies = append(companies, scraper.Company{
				Name:  c.Name,
				Slug:  c.Slug,
				Board: scraper.BoardType(c.Board),
			})
		}

		allJobs := scraper.ScrapeAll(companies)
		slog.Info("scraping complete", "total_jobs", len(allJobs))

		// Persist all scraped jobs and filter to only unsent ones.
		for i := range allJobs {
			if err := db.UpsertJob(&allJobs[i]); err != nil {
				slog.Warn("upsert job failed", "id", allJobs[i].ID, "error", err)
			}
		}

		newJobs, err := db.FilterNewJobs(allJobs)
		if err != nil {
			slog.Error("filter new jobs failed", "error", err)
			return
		}
		slog.Info("deduplication complete", "new_jobs", len(newJobs), "total_jobs", len(allJobs))

		jobs := matcher.FilterByLocation(newJobs)
		slog.Info("location filter applied", "remaining_jobs", len(jobs))

		// Load learned adjustments for this run.
		if err := learner.LoadCache(db); err != nil {
			slog.Warn("learner cache load failed, scoring without adjustments", "error", err)
		}

		matched := matcher.RankJobs(jobs, profile, cfg.Scoring.PrioritySkills, cfg.Scoring.TargetTitles, learner.ScoreAdjustment)
		metrics.JobsMatchedTotal.Add(float64(len(matched)))
		slog.Info("ranking complete", "matched_jobs", len(matched))

		if len(matched) == 0 {
			slog.Info("no new jobs matched, skipping email")
			return
		}

		top := matched
		if len(top) > cfg.TopResults {
			top = top[:cfg.TopResults]
		}

		if cfg.GmailAppPassword == "" {
			slog.Info("skipping email (no GMAIL_APP_PASSWORD)")
			for _, j := range top[:min(5, len(top))] {
				slog.Info("top job", "title", j.Title, "company", j.Company, "score", j.Score)
			}
			return
		}

		emailCfg := email.Config{
			To:       cfg.Email.To,
			From:     cfg.Email.From,
			SMTPHost: cfg.Email.SMTPHost,
			SMTPPort: cfg.Email.SMTPPort,
			Password: cfg.GmailAppPassword,
		}

		if err := email.Send(emailCfg, top, cfg.FeedbackBaseURL); err != nil {
			slog.Error("failed to send email", "error", err)
			return
		}

		if err := db.MarkSent(top, time.Now()); err != nil {
			slog.Warn("mark sent failed", "error", err)
		}

		metrics.EmailsSentTotal.Inc()
		slog.Info("email sent", "to", cfg.Email.To, "jobs", len(top))
	}

	if *runNow {
		job()
		return
	}

	c := cron.New()
	if _, err := c.AddFunc(cfg.Schedule, job); err != nil {
		slog.Error("invalid cron schedule", "schedule", cfg.Schedule, "error", err)
		os.Exit(1)
	}
	c.Start()
	slog.Info("scheduler started", "schedule", cfg.Schedule)

	// Start feedback HTTP server.
	go func() {
		if err := feedback.ListenAndServe(cfg.FeedbackPort, db); err != nil {
			slog.Error("feedback server failed", "error", err)
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	addr := fmt.Sprintf(":%d", cfg.MetricsPort)
	slog.Info("metrics server listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("metrics server failed", "error", err)
		os.Exit(1)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
