package memory

import "testing"

// Filler words would match nearly every memory, so exact-word search ignores
// them unless the query has nothing else.
func TestExactWordSearchIgnoresFillerWords(t *testing.T) {
	for query, want := range map[string]string{
		"the weather in paris":       `"weather" OR "paris"`,
		"How often are backups made": `"often" OR "backups" OR "made"`,
		"the":                        `"the"`,
		"goVet":                      `"govet" OR "go" OR "vet"`,
		"isReady":                    `"isready" OR "ready"`,
	} {
		if got := prepareFTSQuery(query); got != want {
			t.Errorf("%q: got %s, want %s", query, got, want)
		}
	}
}
