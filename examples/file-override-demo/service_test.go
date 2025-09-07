//go:build examples
// +build examples

package main

import (
	"fmt"
	"testing"
)

func TestProcessData(t *testing.T) {
	// Both should be allowed in test files
	fmt.Println("Running test...")
	ProcessData()
}
