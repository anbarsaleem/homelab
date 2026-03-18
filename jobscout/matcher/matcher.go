package matcher

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/anbarsaleem/homelab/jobscout/resume"
	"github.com/anbarsaleem/homelab/jobscout/scraper"
)

// seniorityOrder maps seniority keywords to a numeric level.
var seniorityOrder = map[string]int{
	"intern":    0,
	"junior":    1,
	"mid":       2,
	"senior":    3,
	"staff":     4,
	"principal": 5,
	"director":  6,
}

// yoePatterns extracts the minimum years of experience stated in a job description.
var yoePatterns = []*regexp.Regexp{
	// "5+ years of experience", "5 years experience", "5+ years of relevant experience"
	regexp.MustCompile(`(\d+)\+?\s*(?:or more\s+)?years?\s*(?:of\s+)?(?:relevant\s+)?(?:professional\s+)?(?:experience|exp\b)`),
	// "minimum of 5 years", "at least 5 years", "minimum 5 years"
	regexp.MustCompile(`(?:minimum|at least|min\.?)\s+(?:of\s+)?(\d+)\s+years?`),
	// "requires 5+ years", "requires 5 years"
	regexp.MustCompile(`requires?\s+(\d+)\+?\s+years?`),
}

// FilterByLocation returns only NYC hybrid and US Remote jobs.
// Empty locations are kept (companies often omit location for remote roles).
func FilterByLocation(jobs []scraper.Job) []scraper.Job {
	var out []scraper.Job
	for _, j := range jobs {
		if isRelevantLocation(j.Location) {
			out = append(out, j)
		}
	}
	return out
}

// nonUSTerms is an exhaustive list of non-US countries, regions, and continents.
// Any remote role whose location contains one of these is rejected.
var nonUSTerms = []string{
	// Regions / blocs
	"europe", "emea", "apac", "latam", "latin america", "asia", "africa",
	"americas", "oceania", "middle east",
	// English-speaking countries often confused with "US remote"
	"uk", "u.k.", "united kingdom", "england", "scotland", "wales",
	"canada", "australia", "new zealand", "ireland",
	// Western Europe
	"spain", "netherlands", "germany", "france", "portugal", "italy",
	"sweden", "norway", "denmark", "finland", "austria", "switzerland",
	"belgium", "luxembourg", "greece", "cyprus",
	// Eastern Europe
	"poland", "czech", "hungary", "romania", "bulgaria", "croatia",
	"slovakia", "slovenia", "serbia", "ukraine", "russia",
	// Asia / Middle East
	"india", "china", "japan", "singapore", "israel", "turkey",
	"south korea", "taiwan", "hong kong", "vietnam", "thailand",
	"philippines", "indonesia", "malaysia", "pakistan",
	// Latin America
	"brazil", "argentina", "colombia", "chile", "mexico", "peru",
	// Africa
	"south africa", "nigeria", "kenya", "egypt",
}

func isRelevantLocation(location string) bool {
	loc := strings.ToLower(strings.TrimSpace(location))

	if loc == "" {
		return true
	}

	// NYC hybrid — always included
	if containsAny(loc, []string{"new york", "nyc", "brooklyn", "manhattan"}) {
		return true
	}

	// Remote roles — only US remote
	if strings.Contains(loc, "remote") {
		return isUSRemote(loc)
	}

	// Bare US-wide entries (no city) — likely fully remote
	if containsAny(loc, []string{"united states", "usa", "u.s.", "u.s.a"}) {
		return true
	}
	if loc == "us" {
		return true
	}

	return false
}

// isUSRemote returns true only if the location contains "remote" and no
// non-US country or region is mentioned.
func isUSRemote(loc string) bool {
	for _, term := range nonUSTerms {
		if strings.Contains(loc, term) {
			return false
		}
	}
	return true
}

func containsAny(s string, keywords []string) bool {
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// RankJobs scores each job and returns them sorted by descending score.
// Jobs scoring 0 are excluded.
func RankJobs(jobs []scraper.Job, profile *resume.Profile, prioritySkills, targetTitles []string) []scraper.Job {
	resumeLevel := inferLevel(profile.YearsExperience)

	for i := range jobs {
		jobs[i].Score = score(&jobs[i], profile, resumeLevel, profile.YearsExperience, prioritySkills, targetTitles)
	}

	var matched []scraper.Job
	for _, j := range jobs {
		if j.Score > 0 {
			matched = append(matched, j)
		}
	}

	sort.Slice(matched, func(i, k int) bool {
		return matched[i].Score > matched[k].Score
	})

	return matched
}

func score(j *scraper.Job, profile *resume.Profile, resumeLevel, resumeYOE int, prioritySkills, targetTitles []string) int {
	title := strings.ToLower(j.Title)
	desc := strings.ToLower(j.Description)
	loc := strings.ToLower(j.Location)
	total := 0

	// Hard filter: if description explicitly requires more YOE than the candidate has,
	// drop the role entirely (allow 1 year of grace).
	if requiredYOE := extractRequiredYOE(desc); requiredYOE > 0 && requiredYOE > resumeYOE+1 {
		return 0
	}

	// Hard filter: Staff/Principal/Director titles are out of reach.
	jobLevel := inferLevelFromTitle(title)
	if jobLevel >= 4 { // staff and above
		return 0
	}

	// Title matching
	for _, target := range targetTitles {
		if title == target {
			total += 10
			break
		}
		if strings.Contains(title, target) {
			total += 5
			break
		}
	}

	// Priority skill matching in description or title
	for _, skill := range prioritySkills {
		if strings.Contains(desc, skill) || strings.Contains(title, skill) {
			total += 3
		}
	}

	// Seniority scoring:
	//   diff=0 (exact match)  → +5
	//   diff=-1 (overqualified by one) → +2
	//   diff=1 (one level up, e.g. mid→senior) → -5
	//   diff>=2 or diff<=-2  → -15 (effectively eliminates the role)
	if jobLevel >= 0 {
		diff := jobLevel - resumeLevel
		switch {
		case diff == 0:
			total += 5
		case diff == -1:
			total += 2
		case diff == 1:
			total -= 5
		default: // diff >= 2 or diff <= -2
			total -= 15
		}
	}

	// Location bonus
	if containsAny(loc, []string{"new york", "nyc", "brooklyn", "manhattan"}) {
		total += 5
	} else if strings.Contains(loc, "remote") {
		if containsAny(loc+" "+desc, []string{"eastern", "east coast", "est ", " et ", "new york"}) {
			total += 3
		}
	}

	if total > 100 {
		total = 100
	}
	if total < 0 {
		total = 0
	}

	return total
}

// extractRequiredYOE finds the highest years-of-experience requirement in a job description.
func extractRequiredYOE(desc string) int {
	max := 0
	for _, re := range yoePatterns {
		for _, match := range re.FindAllStringSubmatch(desc, -1) {
			if len(match) >= 2 {
				if n, err := strconv.Atoi(match[1]); err == nil && n > max {
					max = n
				}
			}
		}
	}
	return max
}

func inferLevel(years int) int {
	switch {
	case years <= 1:
		return 1 // junior
	case years <= 3:
		return 2 // mid
	case years <= 6:
		return 3 // senior
	case years <= 10:
		return 4 // staff
	default:
		return 5 // principal
	}
}

func inferLevelFromTitle(title string) int {
	for keyword, level := range seniorityOrder {
		if strings.Contains(title, keyword) {
			return level
		}
	}
	return -1 // unknown
}
