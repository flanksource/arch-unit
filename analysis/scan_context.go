package analysis

import (
	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/clicky"
)

// ScanContext carries contextual information during dependency scanning
type ScanContext struct {
	*clicky.Task        // Optional task for progress tracking
	ScanRoot     string // The root directory being scanned
	filter       string
	Depth        int
}

// NewScanContext creates a new scan context
func NewScanContext(task *clicky.Task, scanRoot string) *ScanContext {
	return &ScanContext{
		Task:     task,
		ScanRoot: scanRoot,
	}
}

// Infof logs info message if task is available
func (ctx *ScanContext) Infof(format string, args ...interface{}) {
	if ctx != nil && ctx.Task != nil {
		ctx.Task.Infof(format, args...)
	}
}

// Warnf logs warning message if task is available
func (ctx *ScanContext) Warnf(format string, args ...interface{}) {
	if ctx != nil && ctx.Task != nil {
		ctx.Task.Warnf(format, args...)
	}
}

// Debugf logs debug message if task is available
func (ctx *ScanContext) Debugf(format string, args ...interface{}) {
	if ctx != nil && ctx.Task != nil {
		ctx.Task.Debugf(format, args...)
	}
}

func (ctx *ScanContext) Filter(deps []*models.Dependency) []*models.Dependency {
	if ctx.filter == "" {
		return deps
	}
	var filtered []*models.Dependency
	for _, d := range deps {
		if d.Matches(ctx.filter) {
			filtered = append(filtered, d)
		}
	}
	return filtered
}
