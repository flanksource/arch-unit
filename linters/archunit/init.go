package archunit

import (
	"github.com/flanksource/arch-unit/linters"
)

func init() {
	// Register the arch-unit linter with the default registry
	linters.DefaultRegistry.Register(NewArchUnit("."))
}