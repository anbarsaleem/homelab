package feedback

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/anbarsaleem/homelab/jobscout/learner"
	"github.com/anbarsaleem/homelab/jobscout/store"
)

//go:embed confirmation.html
var confirmationFS embed.FS

var confirmTmpl *template.Template

func init() {
	b, err := confirmationFS.ReadFile("confirmation.html")
	if err != nil {
		panic("feedback: cannot read confirmation.html: " + err.Error())
	}
	confirmTmpl = template.Must(template.New("confirm").Parse(string(b)))
}

type confirmData struct {
	Title      string
	Company    string
	ActionLabel string
	UndoURL    string
}

var actionLabels = map[string]string{
	"like":      "Applied",
	"superlike": "More Like This",
	"dislike":   "Not Relevant",
}

// ListenAndServe starts the feedback HTTP server on the given port.
func ListenAndServe(port int, s *store.Store) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/feedback", feedbackHandler(s))
	mux.HandleFunc("/feedback/undo", undoHandler(s))

	addr := fmt.Sprintf(":%d", port)
	slog.Info("feedback server listening", "addr", addr)
	return http.ListenAndServe(addr, mux)
}

func feedbackHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		id := q.Get("id")
		board := q.Get("board")
		action := q.Get("action")

		if id == "" || board == "" {
			http.Error(w, "missing id or board", http.StatusBadRequest)
			return
		}

		validActions := map[string]bool{"like": true, "dislike": true, "superlike": true}
		if !validActions[action] {
			http.Error(w, "invalid action", http.StatusBadRequest)
			return
		}

		job, err := s.GetJob(id, board)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			slog.Error("feedback: get job failed", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if err := s.RecordFeedback(id, board, action); err != nil {
			slog.Error("feedback: record failed", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		slog.Info("feedback recorded", "id", id, "board", board, "action", action)

		// Recompute learned signals after each feedback.
		go func() {
			if err := learner.Recompute(s); err != nil {
				slog.Warn("learner recompute failed", "error", err)
			}
		}()

		label := actionLabels[action]
		undoURL := fmt.Sprintf("/feedback/undo?id=%s&board=%s", id, board)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		confirmTmpl.Execute(w, confirmData{
			Title:       job.Title,
			Company:     job.Company,
			ActionLabel: label,
			UndoURL:     undoURL,
		})
	}
}

func undoHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		id := q.Get("id")
		board := q.Get("board")

		if id == "" || board == "" {
			http.Error(w, "missing id or board", http.StatusBadRequest)
			return
		}

		if err := s.ClearFeedback(id, board); err != nil {
			slog.Error("feedback: clear failed", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		slog.Info("feedback cleared", "id", id, "board", board)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintln(w, `<!DOCTYPE html><html><body><p>Feedback removed. <a href="javascript:history.back()">Go back</a></p></body></html>`)
	}
}
