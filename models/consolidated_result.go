package models

import (
	"time"
)

// ConsolidatedResult represents the combined results from arch-unit analysis and linters
type ConsolidatedResult struct {
	Summary    ConsolidatedSummary `json:"summary"`
	ArchUnit   *AnalysisResult     `json:"arch_unit"`
	Linters    []LinterResult      `json:"linters"`
	Violations []Violation         `json:"violations"`
	Timestamp  time.Time           `json:"timestamp"`
}

// ConsolidatedSummary provides aggregate statistics across all tools
type ConsolidatedSummary struct {
	FilesAnalyzed     int           `json:"files_analyzed"`
	RulesApplied      int           `json:"rules_applied"`
	LintersRun        int           `json:"linters_run"`
	LintersSuccessful int           `json:"linters_successful"`
	TotalViolations   int           `json:"total_violations"`
	ArchViolations    int           `json:"arch_violations"`
	LinterViolations  int           `json:"linter_violations"`
	Duration          time.Duration `json:"duration"`
}

// LinterResult represents the result of running a linter (imported to avoid circular dependency)
type LinterResult struct {
	Linter     string        `json:"linter"`
	Success    bool          `json:"success"`
	Duration   time.Duration `json:"duration"`
	Violations []Violation   `json:"violations"`
	RawOutput  string        `json:"raw_output,omitempty"`
	Error      string        `json:"error,omitempty"`
	FileCount  int           `json:"file_count,omitempty"`
	RuleCount  int           `json:"rule_count,omitempty"`
}

// NewConsolidatedResult creates a new consolidated result from arch-unit and linter results
func NewConsolidatedResult(archResult *AnalysisResult, linterResults []LinterResult) *ConsolidatedResult {
	start := time.Now()

	result := &ConsolidatedResult{
		ArchUnit:  archResult,
		Linters:   linterResults,
		Timestamp: start,
	}

	// Consolidate all violations
	result.consolidateViolations()

	// Calculate summary
	result.GenerateSummary()

	return result
}

// consolidateViolations merges violations from arch-unit and all linters
func (cr *ConsolidatedResult) consolidateViolations() {
	var allViolations []Violation

	// Add arch-unit violations with source
	if cr.ArchUnit != nil {
		for _, v := range cr.ArchUnit.Violations {
			// Only set source if not already set (for database-fetched violations)
			if v.Source == "" {
				v.Source = "arch-unit"
			}
			allViolations = append(allViolations, v)
		}
	}

	// Add linter violations with source
	for _, linterResult := range cr.Linters {
		for _, v := range linterResult.Violations {
			if v.Source == "" {
				v.Source = linterResult.Linter
			}
			allViolations = append(allViolations, v)
		}
	}

	// Don't deduplicate - keep all violations to show which tools detected them
	cr.Violations = allViolations
}

// GenerateSummary creates a summary of the analysis results
func (cr *ConsolidatedResult) GenerateSummary() {
	summary := ConsolidatedSummary{}

	if cr.ArchUnit != nil {
		summary.FilesAnalyzed = cr.ArchUnit.FileCount
		summary.RulesApplied = cr.ArchUnit.RuleCount
	} else {
		// When archResult is nil, aggregate counts from linter results
		for _, linterResult := range cr.Linters {
			if linterResult.FileCount > 0 {
				summary.FilesAnalyzed += linterResult.FileCount
			}
			if linterResult.RuleCount > 0 {
				summary.RulesApplied += linterResult.RuleCount
			}
		}
	}

	// Linter statistics
	summary.LintersRun = len(cr.Linters)
	for _, linterResult := range cr.Linters {
		if linterResult.Success {
			summary.LintersSuccessful++
		}
	}

	// Count violations by source
	for _, v := range cr.Violations {
		if v.Source == "arch-unit" || v.Source == "" {
			summary.ArchViolations++
		} else {
			summary.LinterViolations++
		}
	}

	// Total violations
	summary.TotalViolations = len(cr.Violations)

	cr.Summary = summary
}

// GetViolationsByFile returns violations grouped by file
func (cr *ConsolidatedResult) GetViolationsByFile() map[string][]Violation {
	violationsByFile := make(map[string][]Violation)

	for _, violation := range cr.Violations {
		violationsByFile[violation.File] = append(violationsByFile[violation.File], violation)
	}

	return violationsByFile
}

// GetViolationsByType returns violations grouped by their source (arch-unit vs linter)
func (cr *ConsolidatedResult) GetViolationsByType() map[string][]Violation {
	violationsByType := make(map[string][]Violation)

	for _, violation := range cr.Violations {
		source := "arch-unit"
		// Use the violation's Source field instead of Called field for linter detection
		if violation.Source != "" {
			source = violation.Source
		}

		violationsByType[source] = append(violationsByType[source], violation)
	}

	return violationsByType
}

// GetFailedLinters returns the names of linters that failed
func (cr *ConsolidatedResult) GetFailedLinters() []string {
	var failed []string

	for _, linterResult := range cr.Linters {
		if !linterResult.Success {
			failed = append(failed, linterResult.Linter)
		}
	}

	return failed
}

// HasViolations returns true if there are any violations from any source
func (cr *ConsolidatedResult) HasViolations() bool {
	return len(cr.Violations) > 0
}

// HasFailures returns true if there are violations or linter failures
func (cr *ConsolidatedResult) HasFailures() bool {
	return cr.HasViolations() || len(cr.GetFailedLinters()) > 0
}
