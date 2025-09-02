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
	MaxDepth     int
	ShowIndirect bool
}

// NewScanContext creates a new scan context
func NewScanContext(task *clicky.Task, scanRoot string) *ScanContext {
	return &ScanContext{
		Task:     task,
		ScanRoot: scanRoot,
	}
}

// WithDepth sets the depth on the scan context
func (ctx *ScanContext) WithDepth(depth int) *ScanContext {
	ctx.Depth = depth
	return ctx
}

// WithIndirect sets whether to show indirect dependencies
func (ctx *ScanContext) WithIndirect(showIndirect bool) *ScanContext {
	ctx.ShowIndirect = showIndirect
	return ctx
}

// WithFilter sets the filter on the scan context
func (ctx *ScanContext) WithFilter(filter string) *ScanContext {
	ctx.filter = filter
	return ctx
}

// FilterDeps is an alias for Filter for compatibility
func (ctx *ScanContext) Matches(dep *models.Dependency) bool {
	if ctx == nil {
		return true // If no context, don't filter
	}
	return dep.Matches(ctx.filter)
}

// FilterDeps is an alias for Filter for compatibility
func (ctx *ScanContext) FilterDeps(deps []*models.Dependency) []*models.Dependency {
	return ctx.Filter(deps)
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
	if ctx == nil || ctx.filter == "" {
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
