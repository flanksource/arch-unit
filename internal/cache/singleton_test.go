package cache

import (
	"testing"
)

func TestASTCacheSingleton(t *testing.T) {
	// Reset singleton for testing
	ResetASTCache()

	// First call should create instance
	cache1, err := GetASTCache()
	if err != nil {
		t.Fatalf("Failed to get AST cache: %v", err)
	}
	if cache1 == nil {
		t.Fatal("Expected non-nil cache")
	}

	// Second call should return same instance
	cache2, err := GetASTCache()
	if err != nil {
		t.Fatalf("Failed to get AST cache second time: %v", err)
	}

	if cache1 != cache2 {
		t.Error("Expected same instance from singleton")
	}
}

func TestViolationCacheSingleton(t *testing.T) {
	// Reset singleton for testing
	ResetViolationCache()

	// First call should create instance
	cache1, err := GetViolationCache()
	if err != nil {
		t.Fatalf("Failed to get violation cache: %v", err)
	}
	if cache1 == nil {
		t.Fatal("Expected non-nil cache")
	}

	// Second call should return same instance
	cache2, err := GetViolationCache()
	if err != nil {
		t.Fatalf("Failed to get violation cache second time: %v", err)
	}

	if cache1 != cache2 {
		t.Error("Expected same instance from singleton")
	}
}
