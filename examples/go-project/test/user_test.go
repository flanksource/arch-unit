//go:build examples
// +build examples

package test

import (
	"fmt"     // OK: Test can use fmt.Println
	"testing" // OK: Test can use testing package
)

func TestUserService(t *testing.T) {
	// OK: Test can use fmt.Println
	fmt.Println("Running user service tests")

	// OK: Test can access internal packages
	// cfg := config.Load()
	// fmt.Printf("Config loaded: %+v\n", cfg)

	// svc := service.NewService()
	// if svc == nil {
	// 	t.Fatal("Service should not be nil")
	// }
}

// OK: Test methods are allowed in test directory
func TestMockUser(t *testing.T) {
	// Mock implementation for testing
	fmt.Println("Testing mock user")
}

// OK: Mock functions are allowed in test directory
func MockUserRepository() *MockRepo {
	return &MockRepo{}
}

type MockRepo struct{}

func (m *MockRepo) TestHelper() {
	// OK: Test helper methods allowed
	fmt.Println("Test helper")
}
