package fixtures_test

import (
	"path/filepath"
	"testing"

	"github.com/flanksource/arch-unit/tests/fixtures"
)

func TestParseMarkdownFixtures(t *testing.T) {
	// Test parsing the ast_queries.md file
	fixturesDir := "."
	fixtureFile := filepath.Join(fixturesDir, "ast_queries.md")
	
	tests, err := fixtures.ParseMarkdownFixtures(fixtureFile)
	if err != nil {
		t.Fatalf("Failed to parse fixtures: %v", err)
	}
	
	if len(tests) == 0 {
		t.Fatal("No test fixtures were parsed")
	}
	
	t.Logf("Parsed %d test fixtures", len(tests))
	
	// Check first test has expected fields
	first := tests[0]
	if first.Name == "" {
		t.Error("First test has no name")
	}
	if first.CWD == "" {
		t.Error("First test has no CWD")
	}
	if first.Query == "" {
		t.Error("First test has no query")
	}
	if first.CEL == "" {
		t.Error("First test has no CEL validation")
	}
	
	t.Logf("First test: Name=%s, CWD=%s, Query=%s", first.Name, first.CWD, first.Query)
}