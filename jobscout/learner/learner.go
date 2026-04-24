package learner

import (
	"log/slog"
	"math"
	"strings"
	"unicode"

	"github.com/anbarsaleem/homelab/jobscout/scraper"
	"github.com/anbarsaleem/homelab/jobscout/store"
)

// stopWords is a set of common English words to ignore during tokenization.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "was": true,
	"you": true, "with": true, "this": true, "that": true, "from": true,
	"have": true, "will": true, "our": true, "your": true, "they": true,
	"their": true, "what": true, "who": true, "how": true, "all": true,
	"can": true, "not": true, "but": true, "more": true, "also": true,
	"its": true, "been": true, "has": true, "than": true, "into": true,
	"we": true, "be": true, "to": true, "of": true, "in": true,
	"on": true, "at": true, "by": true, "as": true, "or": true,
	"an": true, "is": true, "it": true, "if": true, "do": true,
	"a": true, "i": true,
}

// tokenize lowercases text and extracts words ≥ 3 chars, excluding stop words.
func tokenize(text string) []string {
	lower := strings.ToLower(text)
	var tokens []string
	var cur strings.Builder

	flush := func() {
		w := cur.String()
		cur.Reset()
		if len(w) >= 3 && !stopWords[w] {
			tokens = append(tokens, w)
		}
	}

	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

// titleBigrams returns 2-word fragments from a job title (lowercased).
func titleBigrams(title string) []string {
	words := strings.Fields(strings.ToLower(title))
	var bigrams []string
	for i := 0; i+1 < len(words); i++ {
		bigrams = append(bigrams, words[i]+" "+words[i+1])
	}
	return bigrams
}

// Recompute rebuilds the learned_terms and learned_signals tables from all feedback.
func Recompute(s *store.Store) error {
	liked, disliked, superliked, err := s.GetFeedbackJobs()
	if err != nil {
		return err
	}

	total := len(liked) + len(disliked) + len(superliked)
	if total == 0 {
		return nil
	}

	// Scale factor: ramp up from 0 to 1 as feedback grows to 10.
	scale := math.Min(float64(total), 10.0) / 10.0

	// --- Term affinity ---
	likedFreq := map[string]float64{}
	dislikedFreq := map[string]float64{}
	superlikedFreq := map[string]float64{}

	countTerms := func(jobs []store.JobRecord, freq map[string]float64) {
		for _, j := range jobs {
			for _, t := range tokenize(j.Description + " " + j.Title) {
				freq[t]++
			}
		}
	}
	countTerms(liked, likedFreq)
	countTerms(disliked, dislikedFreq)
	countTerms(superliked, superlikedFreq)

	// Collect all unique terms.
	allTerms := map[string]bool{}
	for t := range likedFreq {
		allTerms[t] = true
	}
	for t := range dislikedFreq {
		allTerms[t] = true
	}
	for t := range superlikedFreq {
		allTerms[t] = true
	}

	for term := range allTerms {
		affinity := (likedFreq[term]*1.0 + superlikedFreq[term]*2.0) - (dislikedFreq[term] * 1.5)
		// Normalize by total feedback count.
		if total > 0 {
			affinity /= float64(total)
		}
		affinity *= scale
		count := int(likedFreq[term] + dislikedFreq[term] + superlikedFreq[term])
		if err := s.UpsertLearnedTerm(term, affinity, count); err != nil {
			slog.Warn("learner: upsert term failed", "term", term, "error", err)
		}
	}

	// --- Company signals ---
	type companyStats struct {
		likes, dislikes, superlikes int
	}
	companies := map[string]*companyStats{}

	for _, j := range liked {
		c := strings.ToLower(j.Company)
		if companies[c] == nil {
			companies[c] = &companyStats{}
		}
		companies[c].likes++
	}
	for _, j := range disliked {
		c := strings.ToLower(j.Company)
		if companies[c] == nil {
			companies[c] = &companyStats{}
		}
		companies[c].dislikes++
	}
	for _, j := range superliked {
		c := strings.ToLower(j.Company)
		if companies[c] == nil {
			companies[c] = &companyStats{}
		}
		companies[c].superlikes++
	}

	for company, st := range companies {
		total := st.likes + st.dislikes + st.superlikes
		score := (float64(st.likes) + 2.0*float64(st.superlikes) - 1.5*float64(st.dislikes)) / float64(total)
		score *= scale
		if err := s.UpsertLearnedSignal("company", company, score, st.likes+st.superlikes, st.dislikes); err != nil {
			slog.Warn("learner: upsert company signal failed", "company", company, "error", err)
		}
	}

	// --- Title bigram signals ---
	type bigramStats struct {
		likes, dislikes, superlikes int
	}
	bigrams := map[string]*bigramStats{}

	addBigrams := func(jobs []store.JobRecord, field string) {
		for _, j := range jobs {
			for _, bg := range titleBigrams(j.Title) {
				if bigrams[bg] == nil {
					bigrams[bg] = &bigramStats{}
				}
				switch field {
				case "like":
					bigrams[bg].likes++
				case "dislike":
					bigrams[bg].dislikes++
				case "superlike":
					bigrams[bg].superlikes++
				}
			}
		}
	}
	addBigrams(liked, "like")
	addBigrams(disliked, "dislike")
	addBigrams(superliked, "superlike")

	for bg, st := range bigrams {
		total := st.likes + st.dislikes + st.superlikes
		score := (float64(st.likes) + 2.0*float64(st.superlikes) - 1.5*float64(st.dislikes)) / float64(total)
		score *= scale
		if err := s.UpsertLearnedSignal("title_fragment", bg, score, st.likes+st.superlikes, st.dislikes); err != nil {
			slog.Warn("learner: upsert title signal failed", "bigram", bg, "error", err)
		}
	}

	slog.Info("learner: recomputed", "total_feedback", total, "terms", len(allTerms), "companies", len(companies), "bigrams", len(bigrams))
	return nil
}

// cache holds a snapshot of learned data for one scoring run.
type cache struct {
	terms   map[string]float64
	signals []store.LearnedSignal
}

var runCache *cache

// LoadCache pre-loads learned data into memory for the duration of a scoring run.
// Call this once at the start of each run, then use ScoreAdjustment.
func LoadCache(s *store.Store) error {
	terms, err := s.GetLearnedTerms()
	if err != nil {
		return err
	}
	signals, err := s.GetLearnedSignals()
	if err != nil {
		return err
	}
	runCache = &cache{terms: terms, signals: signals}
	return nil
}

// ScoreAdjustment returns a learned score adjustment for a job, capped at ±25.
// Returns 0 if no cache is loaded (cold start).
func ScoreAdjustment(j *scraper.Job) int {
	if runCache == nil {
		return 0
	}

	// Term bonus: sum term affinities for tokens in description + title, cap at ±15.
	termBonus := 0.0
	for _, t := range tokenize(j.Description + " " + j.Title) {
		termBonus += runCache.terms[t]
	}
	termBonus = clamp(termBonus, -15, 15)

	// Company and title-fragment bonuses.
	companyBonus := 0.0
	titleBonus := 0.0
	jCompany := strings.ToLower(j.Company)
	jBigrams := titleBigrams(j.Title)

	for _, sig := range runCache.signals {
		switch sig.SignalType {
		case "company":
			if sig.SignalValue == jCompany {
				companyBonus = clamp(sig.Score*10, -10, 10)
			}
		case "title_fragment":
			for _, bg := range jBigrams {
				if sig.SignalValue == bg {
					titleBonus += sig.Score * 10
				}
			}
		}
	}
	titleBonus = clamp(titleBonus, -10, 10)

	total := termBonus + companyBonus + titleBonus
	return int(clamp(total, -25, 25))
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
