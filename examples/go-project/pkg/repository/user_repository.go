package repository

import (
	"database/sql" // OK: Repository can access database
	"log"
)

type UserRepository struct {
	db *sql.DB // OK: Repository can have database reference
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetByID(id string) (*User, error) {
	// OK: Repository can execute SQL queries
	row := r.db.QueryRow("SELECT id, name FROM users WHERE id = ?", id)

	// This is fine - using logger
	log.Printf("Fetching user from database: %s", id)

	var user User
	err := row.Scan(&user.ID, &user.Name)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *UserRepository) Create(user *User) error {
	// OK: Repository can execute SQL statements
	_, err := r.db.Exec("INSERT INTO users (id, name) VALUES (?, ?)", user.ID, user.Name)
	return err
}

type User struct {
	ID   string
	Name string
}
