package models

import (
	"time"

	"github.com/flanksource/clicky/api"
)

type CommitType string

const (
	CommitTypeFeature  CommitType = "feat"
	CommitTypeBugfix   CommitType = "fix"
	CommitTypeDocs     CommitType = "docs"
	CommitTypeChore    CommitType = "chore"
	CommitTypeRelease  CommitType = "release"
	CommitTypeRefactor CommitType = "refactor"
	CommitTypePerf     CommitType = "perf"
)

type CommitAnalysis struct {
	Type CommitType `json:"type,omitempty"`
	// ExternalSummary is a summary of the commit that would be useful for external consumers who are unfamiliar with the codebase.
	ExternalSummary string `json:"summary,omitempty"`
	// InternalSummary is a summary of the commit that is useful for developers working on the codebase.
	InternalSummary string `json:"internal_summary,omitempty"`
}

type GitCommit struct {
	CommitAnalysis `json:",inline"`
	Hash           string    `json:"hash,omitempty" gorm:"primaryKey"`
	Author         string    `json:"author,omitempty"`
	Email          string    `json:"email,omitempty"`
	Timestamp      time.Time `json:"timestamp,omitempty"`
	Message        string    `json:"message,omitempty"`
	Files          []string  `json:"files,omitempty"`
}

func (c GitCommit) Pretty() api.Text {
	//FIXME implment
	panic("implement")
}

// GitRef is a Git reference (branch, tag) "latest"
type GitRef string

type GitRange struct {
	Start GitRef `json:"start,omitempty"`
	End   GitRef `json:"end,omitempty"`
}

type ReleaseNotesOptions struct {
	IncludeInternalSummary bool         `json:"include_internal_summary,omitempty"`
	IncludeExternalSummary bool         `json:"include_external_summary,omitempty"`
	CommitTypes            []CommitType `json:"commit_types,omitempty"`
	GitRange               `json:",inline"`
}

type Author struct {
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	GithubID string `json:"github_id,omitempty"`
}

type ReleaseNotes struct {
	Options      ReleaseNotesOptions `json:"options,omitempty"`
	Commits      []GitCommit         `json:"commits,omitempty"`
	Contributors []Author            `json:"contributors,omitempty"`
}
