package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/anbarsaleem/homelab/jobscout/scraper"
)

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// JobRecord is a row from the jobs table.
type JobRecord struct {
	ID          string
	Board       string
	Title       string
	Company     string
	Location    string
	Description string
	URL         string
	BaseScore   int
	FinalScore  int
	Feedback    string // "" | "like" | "dislike" | "superlike"
}

// LearnedSignal is a row from the learned_signals table.
type LearnedSignal struct {
	SignalType   string
	SignalValue  string
	Score        float64
	LikeCount    int
	DislikeCount int
}

// Open opens (or creates) the SQLite database at path, creates tables, and enables WAL mode.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migration: %w", err)
	}

	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS jobs (
    id          TEXT NOT NULL,
    board       TEXT NOT NULL,
    title       TEXT NOT NULL,
    company     TEXT NOT NULL,
    location    TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    url         TEXT NOT NULL,
    base_score  INTEGER NOT NULL DEFAULT 0,
    final_score INTEGER NOT NULL DEFAULT 0,
    first_seen  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    sent_at     DATETIME,
    feedback    TEXT,
    feedback_at DATETIME,
    PRIMARY KEY (id, board)
);

CREATE TABLE IF NOT EXISTS learned_terms (
    term       TEXT PRIMARY KEY,
    score      REAL NOT NULL DEFAULT 0.0,
    count      INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS learned_signals (
    signal_type   TEXT NOT NULL,
    signal_value  TEXT NOT NULL,
    score         REAL NOT NULL DEFAULT 0.0,
    like_count    INTEGER NOT NULL DEFAULT 0,
    dislike_count INTEGER NOT NULL DEFAULT 0,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (signal_type, signal_value)
);
`)
	return err
}

// UpsertJob inserts a new job or updates last_seen if it already exists.
func (s *Store) UpsertJob(j *scraper.Job) error {
	_, err := s.db.Exec(`
INSERT INTO jobs (id, board, title, company, location, description, url)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id, board) DO UPDATE SET
    last_seen   = CURRENT_TIMESTAMP,
    title       = excluded.title,
    company     = excluded.company,
    location    = excluded.location,
    description = excluded.description,
    url         = excluded.url
`,
		j.ID, string(j.Board), j.Title, j.Company, j.Location, j.Description, j.URL,
	)
	return err
}

// FilterNewJobs returns only jobs that have never been emailed (sent_at IS NULL).
func (s *Store) FilterNewJobs(jobs []scraper.Job) ([]scraper.Job, error) {
	if len(jobs) == 0 {
		return nil, nil
	}

	var out []scraper.Job
	for _, j := range jobs {
		var sentAt sql.NullTime
		err := s.db.QueryRow(
			`SELECT sent_at FROM jobs WHERE id = ? AND board = ?`,
			j.ID, string(j.Board),
		).Scan(&sentAt)
		if err != nil {
			// Row doesn't exist yet — treat as new.
			if err == sql.ErrNoRows {
				out = append(out, j)
				continue
			}
			return nil, err
		}
		if !sentAt.Valid {
			out = append(out, j)
		}
	}
	return out, nil
}

// MarkSent sets sent_at and final_score on each job.
func (s *Store) MarkSent(jobs []scraper.Job, sentAt time.Time) error {
	for _, j := range jobs {
		_, err := s.db.Exec(
			`UPDATE jobs SET sent_at = ?, final_score = ? WHERE id = ? AND board = ?`,
			sentAt, j.Score, j.ID, string(j.Board),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// RecordFeedback saves feedback for a job.
func (s *Store) RecordFeedback(id, board, action string) error {
	_, err := s.db.Exec(
		`UPDATE jobs SET feedback = ?, feedback_at = CURRENT_TIMESTAMP WHERE id = ? AND board = ?`,
		action, id, board,
	)
	return err
}

// ClearFeedback sets feedback to NULL for a job.
func (s *Store) ClearFeedback(id, board string) error {
	_, err := s.db.Exec(
		`UPDATE jobs SET feedback = NULL, feedback_at = NULL WHERE id = ? AND board = ?`,
		id, board,
	)
	return err
}

// GetJob fetches a single job by id and board.
func (s *Store) GetJob(id, board string) (*JobRecord, error) {
	row := s.db.QueryRow(
		`SELECT id, board, title, company, location, description, url, base_score, final_score,
		        COALESCE(feedback, '')
		 FROM jobs WHERE id = ? AND board = ?`,
		id, board,
	)
	var r JobRecord
	if err := row.Scan(&r.ID, &r.Board, &r.Title, &r.Company, &r.Location,
		&r.Description, &r.URL, &r.BaseScore, &r.FinalScore, &r.Feedback); err != nil {
		return nil, err
	}
	return &r, nil
}

// GetFeedbackJobs returns jobs grouped by feedback type.
func (s *Store) GetFeedbackJobs() (liked, disliked, superliked []JobRecord, err error) {
	rows, err := s.db.Query(`
SELECT id, board, title, company, location, description, url, base_score, final_score,
       COALESCE(feedback, '')
FROM jobs
WHERE feedback IS NOT NULL
`)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var r JobRecord
		if err := rows.Scan(&r.ID, &r.Board, &r.Title, &r.Company, &r.Location,
			&r.Description, &r.URL, &r.BaseScore, &r.FinalScore, &r.Feedback); err != nil {
			return nil, nil, nil, err
		}
		switch r.Feedback {
		case "like":
			liked = append(liked, r)
		case "dislike":
			disliked = append(disliked, r)
		case "superlike":
			superliked = append(superliked, r)
		}
	}
	return liked, disliked, superliked, rows.Err()
}

// UpsertLearnedTerm writes a term score to the learned_terms table.
func (s *Store) UpsertLearnedTerm(term string, score float64, count int) error {
	_, err := s.db.Exec(`
INSERT INTO learned_terms (term, score, count, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(term) DO UPDATE SET
    score      = excluded.score,
    count      = excluded.count,
    updated_at = CURRENT_TIMESTAMP
`, term, score, count)
	return err
}

// UpsertLearnedSignal writes a company/title signal to learned_signals.
func (s *Store) UpsertLearnedSignal(signalType, value string, score float64, likes, dislikes int) error {
	_, err := s.db.Exec(`
INSERT INTO learned_signals (signal_type, signal_value, score, like_count, dislike_count, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(signal_type, signal_value) DO UPDATE SET
    score         = excluded.score,
    like_count    = excluded.like_count,
    dislike_count = excluded.dislike_count,
    updated_at    = CURRENT_TIMESTAMP
`, signalType, value, score, likes, dislikes)
	return err
}

// GetLearnedTerms returns all term → score mappings.
func (s *Store) GetLearnedTerms() (map[string]float64, error) {
	rows, err := s.db.Query(`SELECT term, score FROM learned_terms`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]float64)
	for rows.Next() {
		var term string
		var score float64
		if err := rows.Scan(&term, &score); err != nil {
			return nil, err
		}
		m[term] = score
	}
	return m, rows.Err()
}

// GetLearnedSignals returns all learned signals.
func (s *Store) GetLearnedSignals() ([]LearnedSignal, error) {
	rows, err := s.db.Query(`
SELECT signal_type, signal_value, score, like_count, dislike_count
FROM learned_signals
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LearnedSignal
	for rows.Next() {
		var ls LearnedSignal
		if err := rows.Scan(&ls.SignalType, &ls.SignalValue, &ls.Score,
			&ls.LikeCount, &ls.DislikeCount); err != nil {
			return nil, err
		}
		out = append(out, ls)
	}
	return out, rows.Err()
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}
