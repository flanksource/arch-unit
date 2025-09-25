package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"example.com/app/model"
)

// UserRepository handles user data persistence
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// FindByID retrieves a user by ID (simple method)
func (r *UserRepository) FindByID(id string) (*model.User, error) {
	if id == "" {
		return nil, fmt.Errorf("id cannot be empty")
	}

	query := "SELECT id, name, email, age, status, created_at, updated_at FROM users WHERE id = ?"
	row := r.db.QueryRow(query, id)

	user := &model.User{}
	err := row.Scan(&user.ID, &user.Name, &user.Email, &user.Age, &user.Status, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return user, nil
}

// FindByEmail retrieves a user by email (simple method)
func (r *UserRepository) FindByEmail(email string) (*model.User, error) {
	if email == "" {
		return nil, fmt.Errorf("email cannot be empty")
	}

	query := "SELECT id, name, email, age, status, created_at, updated_at FROM users WHERE email = ?"
	row := r.db.QueryRow(query, email)

	user := &model.User{}
	err := row.Scan(&user.ID, &user.Name, &user.Email, &user.Age, &user.Status, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return user, nil
}

// Save creates a new user (simple method)
func (r *UserRepository) Save(user *model.User) (*model.User, error) {
	if user == nil {
		return nil, fmt.Errorf("user cannot be nil")
	}

	query := `INSERT INTO users (id, name, email, age, status, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.Exec(query, user.ID, user.Name, user.Email, user.Age, user.Status, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// Update updates an existing user (simple method)
func (r *UserRepository) Update(user *model.User) (*model.User, error) {
	if user == nil || user.ID == "" {
		return nil, fmt.Errorf("invalid user for update")
	}

	query := `UPDATE users SET name = ?, email = ?, age = ?, status = ?, updated_at = ? 
		WHERE id = ?`

	result, err := r.db.Exec(query, user.Name, user.Email, user.Age, user.Status, user.UpdatedAt, user.ID)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return user, nil
}

// SearchQuery represents search parameters
type SearchQuery struct {
	NameFilter   *string
	EmailFilter  *string
	StatusFilter *string
	MinAge       *int
	MaxAge       *int
	Limit        int
	Offset       int
	SortBy       string
	SortOrder    string
}

// Search performs complex user search with dynamic query building (high complexity)
func (r *UserRepository) Search(query SearchQuery) ([]*model.User, error) {
	// Build dynamic SQL query
	sqlQuery := "SELECT id, name, email, age, status, created_at, updated_at FROM users"
	var conditions []string
	var args []interface{}

	// Add WHERE clauses based on filters
	if query.NameFilter != nil {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+*query.NameFilter+"%")
	}

	if query.EmailFilter != nil {
		conditions = append(conditions, "email LIKE ?")
		args = append(args, "%"+*query.EmailFilter+"%")
	}

	if query.StatusFilter != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *query.StatusFilter)
	}

	if query.MinAge != nil {
		conditions = append(conditions, "age >= ?")
		args = append(args, *query.MinAge)
	}

	if query.MaxAge != nil {
		conditions = append(conditions, "age <= ?")
		args = append(args, *query.MaxAge)
	}

	// Add WHERE clause if we have conditions
	if len(conditions) > 0 {
		sqlQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	// Add ORDER BY clause
	orderBy := query.SortBy
	if orderBy == "" {
		orderBy = "created_at"
	}

	sortOrder := query.SortOrder
	if sortOrder == "" {
		sortOrder = "DESC"
	}

	// Validate sort fields to prevent SQL injection
	validSortFields := map[string]bool{
		"id": true, "name": true, "email": true, "age": true,
		"status": true, "created_at": true, "updated_at": true,
	}

	if !validSortFields[orderBy] {
		return nil, fmt.Errorf("invalid sort field: %s", orderBy)
	}

	if sortOrder != "ASC" && sortOrder != "DESC" {
		sortOrder = "DESC"
	}

	sqlQuery += fmt.Sprintf(" ORDER BY %s %s", orderBy, sortOrder)

	// Add LIMIT and OFFSET
	sqlQuery += " LIMIT ? OFFSET ?"
	args = append(args, query.Limit, query.Offset)

	// Execute query
	rows, err := r.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var users []*model.User
	for rows.Next() {
		user := &model.User{}
		err := rows.Scan(&user.ID, &user.Name, &user.Email, &user.Age, &user.Status,
			&user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

// CountSearch counts total records matching search criteria (medium complexity)
func (r *UserRepository) CountSearch(query SearchQuery) (int, error) {
	// Build count query with same filters
	sqlQuery := "SELECT COUNT(*) FROM users"
	var conditions []string
	var args []interface{}

	// Same filtering logic as Search method
	if query.NameFilter != nil {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+*query.NameFilter+"%")
	}

	if query.EmailFilter != nil {
		conditions = append(conditions, "email LIKE ?")
		args = append(args, "%"+*query.EmailFilter+"%")
	}

	if query.StatusFilter != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *query.StatusFilter)
	}

	if query.MinAge != nil {
		conditions = append(conditions, "age >= ?")
		args = append(args, *query.MinAge)
	}

	if query.MaxAge != nil {
		conditions = append(conditions, "age <= ?")
		args = append(args, *query.MaxAge)
	}

	// Add WHERE clause if we have conditions
	if len(conditions) > 0 {
		sqlQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	var count int
	row := r.db.QueryRow(sqlQuery, args...)
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// FindActiveUsers finds all active users with pagination (medium complexity)
func (r *UserRepository) FindActiveUsers(limit, offset int) ([]*model.User, error) {
	if limit <= 0 || limit > 1000 {
		return nil, fmt.Errorf("invalid limit: %d", limit)
	}

	if offset < 0 {
		return nil, fmt.Errorf("invalid offset: %d", offset)
	}

	query := `SELECT id, name, email, age, status, created_at, updated_at 
		FROM users 
		WHERE status = 'active' 
		ORDER BY created_at DESC 
		LIMIT ? OFFSET ?`

	rows, err := r.db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var users []*model.User
	for rows.Next() {
		user := &model.User{}
		err := rows.Scan(&user.ID, &user.Name, &user.Email, &user.Age, &user.Status,
			&user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

// GetUserStatistics calculates user statistics (medium complexity)
func (r *UserRepository) GetUserStatistics() (*model.UserStatistics, error) {
	stats := &model.UserStatistics{}

	// Count total users
	err := r.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&stats.TotalUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to count total users: %w", err)
	}

	// Count active users
	err = r.db.QueryRow("SELECT COUNT(*) FROM users WHERE status = 'active'").Scan(&stats.ActiveUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to count active users: %w", err)
	}

	// Count inactive users
	err = r.db.QueryRow("SELECT COUNT(*) FROM users WHERE status = 'inactive'").Scan(&stats.InactiveUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to count inactive users: %w", err)
	}

	// Count suspended users
	err = r.db.QueryRow("SELECT COUNT(*) FROM users WHERE status = 'suspended'").Scan(&stats.SuspendedUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to count suspended users: %w", err)
	}

	// Calculate average age
	var avgAge sql.NullFloat64
	err = r.db.QueryRow("SELECT AVG(age) FROM users WHERE status = 'active'").Scan(&avgAge)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate average age: %w", err)
	}

	if avgAge.Valid {
		stats.AverageAge = avgAge.Float64
	}

	// Get age distribution
	ageQuery := `SELECT 
		COUNT(CASE WHEN age < 18 THEN 1 END) as under_18,
		COUNT(CASE WHEN age >= 18 AND age < 25 THEN 1 END) as age_18_24,
		COUNT(CASE WHEN age >= 25 AND age < 35 THEN 1 END) as age_25_34,
		COUNT(CASE WHEN age >= 35 AND age < 50 THEN 1 END) as age_35_49,
		COUNT(CASE WHEN age >= 50 THEN 1 END) as age_50_plus
		FROM users WHERE status = 'active'`

	row := r.db.QueryRow(ageQuery)
	err = row.Scan(&stats.Under18, &stats.Age18to24, &stats.Age25to34, &stats.Age35to49, &stats.Age50Plus)
	if err != nil {
		return nil, fmt.Errorf("failed to get age distribution: %w", err)
	}

	return stats, nil
}

// BulkInsert inserts multiple users in a single transaction (high complexity)
func (r *UserRepository) BulkInsert(users []*model.User) error {
	if len(users) == 0 {
		return nil
	}

	if len(users) > 1000 {
		return fmt.Errorf("bulk insert limited to 1000 users, got %d", len(users))
	}

	// Start transaction
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// Rollback on error
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Prepare statement
	stmt, err := tx.Prepare(`INSERT INTO users (id, name, email, age, status, created_at, updated_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	// Execute for each user
	for i, user := range users {
		if user == nil {
			return fmt.Errorf("user at index %d is nil", i)
		}

		if user.ID == "" || user.Email == "" {
			return fmt.Errorf("user at index %d has missing required fields", i)
		}

		_, err = stmt.Exec(user.ID, user.Name, user.Email, user.Age, user.Status, user.CreatedAt, user.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to insert user at index %d: %w", i, err)
		}
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// BulkUpdate updates multiple users in a single transaction (high complexity)
func (r *UserRepository) BulkUpdate(users []*model.User) error {
	if len(users) == 0 {
		return nil
	}

	if len(users) > 1000 {
		return fmt.Errorf("bulk update limited to 1000 users, got %d", len(users))
	}

	// Start transaction
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// Rollback on error
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Prepare statement
	stmt, err := tx.Prepare(`UPDATE users SET name = ?, email = ?, age = ?, status = ?, updated_at = ? 
		WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	// Execute for each user
	for i, user := range users {
		if user == nil {
			return fmt.Errorf("user at index %d is nil", i)
		}

		if user.ID == "" {
			return fmt.Errorf("user at index %d has empty ID", i)
		}

		result, execErr := stmt.Exec(user.Name, user.Email, user.Age, user.Status, user.UpdatedAt, user.ID)
		if execErr != nil {
			return fmt.Errorf("failed to update user at index %d: %w", i, execErr)
		}

		// Check if user was actually updated
		rowsAffected, raErr := result.RowsAffected()
		if raErr != nil {
			return fmt.Errorf("failed to get rows affected for user at index %d: %w", i, raErr)
		}

		if rowsAffected == 0 {
			return fmt.Errorf("user at index %d not found", i)
		}
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DeleteInactive soft deletes inactive users older than specified days (medium complexity)
func (r *UserRepository) DeleteInactive(olderThanDays int) (int, error) {
	if olderThanDays <= 0 {
		return 0, fmt.Errorf("olderThanDays must be positive")
	}

	query := `UPDATE users SET status = 'deleted', updated_at = ? 
		WHERE status = 'inactive' AND updated_at < ?`

	// Calculate cutoff timestamp
	cutoffTime := getCurrentTimestamp() - int64(olderThanDays*24*60*60)

	result, err := r.db.Exec(query, getCurrentTimestamp(), cutoffTime)
	if err != nil {
		return 0, fmt.Errorf("failed to delete inactive users: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rowsAffected), nil
}

// Helper functions
func getCurrentTimestamp() int64 {
	return 1640995200 // Mock timestamp
}
