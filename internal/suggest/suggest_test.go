package suggest

import (
	"reflect"
	"testing"
)

func TestSimilar(t *testing.T) {
	tests := []struct {
		name       string
		lookup     string
		candidates []string
		n          int
		want       []string
	}{
		{
			name:       "typo within threshold",
			lookup:     "prdo",
			candidates: []string{"prod", "gcp:staging", "unrelated"},
			n:          3,
			want:       []string{"prod"},
		},
		{
			name:       "typo ties with provider suffix break alphabetically",
			lookup:     "prdo",
			candidates: []string{"prod", "aws:prod"},
			n:          3,
			want:       []string{"aws:prod", "prod"},
		},
		{
			name:       "prefix match",
			lookup:     "db",
			candidates: []string{"db-prod", "cache"},
			n:          3,
			want:       []string{"db-prod"},
		},
		{
			name:       "case-insensitive equal",
			lookup:     "PROD",
			candidates: []string{"prod", "staging"},
			n:          3,
			want:       []string{"prod"},
		},
		{
			name:       "provider suffix match",
			lookup:     "prod",
			candidates: []string{"aws:prod", "gcp:staging"},
			n:          3,
			want:       []string{"aws:prod"},
		},
		{
			name:       "full name beats provider suffix",
			lookup:     "aws:prd",
			candidates: []string{"aws:prod"},
			n:          3,
			want:       []string{"aws:prod"},
		},
		{
			name:       "prefix via provider suffix",
			lookup:     "pro",
			candidates: []string{"aws:prod"},
			n:          3,
			want:       []string{"aws:prod"},
		},
		{
			name:       "equal ranks first",
			lookup:     "prod",
			candidates: []string{"prod2", "PROD"},
			n:          3,
			want:       []string{"PROD", "prod2"},
		},
		{
			name:       "smaller distance ranks first",
			lookup:     "web",
			candidates: []string{"web-server", "wex", "WEB-1", "web-2"},
			n:          3,
			want:       []string{"wex", "WEB-1", "web-2"},
		},
		{
			name:       "alphabetical tiebreak",
			lookup:     "ab",
			candidates: []string{"abz", "abx"},
			n:          3,
			want:       []string{"abx", "abz"},
		},
		{
			name:       "limit n",
			lookup:     "web",
			candidates: []string{"web-1", "web-2", "web-3", "wex"},
			n:          2,
			want:       []string{"wex", "web-1"},
		},
		{
			name:       "limit one",
			lookup:     "web",
			candidates: []string{"web-1", "web-2", "wex"},
			n:          1,
			want:       []string{"wex"},
		},
		{
			name:       "n larger than matches returns all",
			lookup:     "prdo",
			candidates: []string{"prod", "unrelated"},
			n:          5,
			want:       []string{"prod"},
		},
		{
			name:       "exact match excluded",
			lookup:     "prod",
			candidates: []string{"prod", "prod-2"},
			n:          3,
			want:       []string{"prod-2"},
		},
		{
			name:       "only the exact match excludes everything",
			lookup:     "prod",
			candidates: []string{"prod"},
			n:          3,
			want:       nil,
		},
		{
			name:       "duplicates dropped",
			lookup:     "prdo",
			candidates: []string{"prod", "prod", "aws:prod", "prod"},
			n:          3,
			want:       []string{"aws:prod", "prod"},
		},
		{
			name:       "no match",
			lookup:     "zzzzzz",
			candidates: []string{"prod", "staging"},
			n:          3,
			want:       nil,
		},
		{
			name:       "distance over threshold without prefix",
			lookup:     "ab",
			candidates: []string{"axby"},
			n:          3,
			want:       nil,
		},
		{
			name:       "empty lookup",
			lookup:     "",
			candidates: []string{"prod"},
			n:          3,
			want:       nil,
		},
		{
			name:       "no candidates",
			lookup:     "prod",
			candidates: nil,
			n:          3,
			want:       nil,
		},
		{
			name:       "zero limit",
			lookup:     "prod",
			candidates: []string{"prod-2"},
			n:          0,
			want:       nil,
		},
		{
			name:       "rune-based distance",
			lookup:     "café",
			candidates: []string{"cafe", "unrelated"},
			n:          3,
			want:       []string{"cafe"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Similar(tt.lookup, tt.candidates, tt.n)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Similar(%q, %v, %d) = %v, want %v", tt.lookup, tt.candidates, tt.n, got, tt.want)
			}
		})
	}
}

func TestDidYouMean(t *testing.T) {
	tests := []struct {
		name    string
		matches []string
		want    string
	}{
		{name: "no matches", matches: nil, want: ""},
		{name: "empty matches", matches: []string{}, want: ""},
		{name: "one match", matches: []string{"prod"}, want: `did you mean "prod"?`},
		{name: "two matches", matches: []string{"prod", "aws:prod"}, want: `did you mean "prod" or "aws:prod"?`},
		{name: "three matches", matches: []string{"web-1", "web-2", "web-3"}, want: `did you mean "web-1", "web-2" or "web-3"?`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DidYouMean(tt.matches); got != tt.want {
				t.Errorf("DidYouMean(%v) = %q, want %q", tt.matches, got, tt.want)
			}
		})
	}
}
