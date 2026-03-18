package scraper

// BoardType identifies which career platform a company uses.
type BoardType string

const (
	BoardGreenhouse BoardType = "greenhouse"
	BoardLever      BoardType = "lever"
)

// Company represents a target employer with its job board configuration.
type Company struct {
	Name  string
	Slug  string
	Board BoardType
}

// Job is a normalized job posting from any platform.
type Job struct {
	ID          string
	Title       string
	Company     string
	Location    string
	Description string
	URL         string
	Score       int // set by matcher
}
