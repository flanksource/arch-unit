package service

import (
	"database/sql" // VIOLATION: Direct database access outside repository
	"fmt"
	"log"
	"net/http" // VIOLATION: Direct HTTP client usage
)

type UserService struct {
	db *sql.DB // VIOLATION: Direct database reference
}

func NewService() *UserService {
	return &UserService{}
}

func (s *UserService) GetUser(id string) (*User, error) {
	// VIOLATION: Direct database query outside repository
	row := s.db.QueryRow("SELECT * FROM users WHERE id = ?", id)
	_ = row // Placeholder to avoid unused variable error

	// VIOLATION: Using fmt.Printf
	fmt.Printf("Fetching user %s\n", id)

	// This is fine - using logger
	log.Printf("User fetch requested for ID: %s", id)

	// VIOLATION: Direct HTTP call
	resp, err := http.Get("https://api.example.com/user/" + id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	return &User{ID: id}, nil
}

// VIOLATION: Test method in production code
func (s *UserService) TestUserCreation() {
	// Test code shouldn't be in production
}

// VIOLATION: Mock in production code
func (s *UserService) MockUser() *User {
	return &User{ID: "mock"}
}

type User struct {
	ID   string
	Name string
}
