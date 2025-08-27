package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/app/model"
	"example.com/app/repository"
)

var (
	ErrUserExists   = errors.New("user already exists")
	ErrUserNotFound = errors.New("user not found")
	ErrInvalidUser  = errors.New("invalid user data")
)

// UserService handles user business logic
type UserService struct {
	userRepo     *repository.UserRepository
	emailService *EmailService
	auditLog     *AuditLogger
}

// NewUserService creates a new UserService
func NewUserService(userRepo *repository.UserRepository, emailService *EmailService, auditLog *AuditLogger) *UserService {
	return &UserService{
		userRepo:     userRepo,
		emailService: emailService,
		auditLog:     auditLog,
	}
}

// GetUserByID retrieves a user by ID (simple method)
func (s *UserService) GetUserByID(id string) (*model.User, error) {
	if id == "" {
		return nil, ErrInvalidUser
	}

	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, err
	}

	if user == nil {
		return nil, ErrUserNotFound
	}

	return user, nil
}

// GetUserByEmail retrieves a user by email (simple method)
func (s *UserService) GetUserByEmail(email string) (*model.User, error) {
	if email == "" {
		return nil, ErrInvalidUser
	}

	return s.userRepo.FindByEmail(email)
}

// CreateUser creates a new user with validation and side effects (medium complexity)
func (s *UserService) CreateUser(req *model.CreateUserRequest) (*model.User, error) {
	// Validation
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Check if user already exists
	existing, _ := s.userRepo.FindByEmail(req.Email)
	if existing != nil {
		return nil, ErrUserExists
	}

	// Create user model
	user := &model.User{
		ID:        generateID(),
		Name:      req.Name,
		Email:     req.Email,
		Age:       req.Age,
		Status:    model.StatusActive,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}

	// Save to repository
	savedUser, err := s.userRepo.Save(user)
	if err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	// Side effects
	if err := s.sendWelcomeEmail(savedUser); err != nil {
		// Log error but don't fail the operation
		s.auditLog.LogError("Failed to send welcome email", err)
	}

	s.auditLog.LogUserCreated(savedUser.ID, savedUser.Email)

	return savedUser, nil
}

// UpdateUser updates an existing user (medium complexity)
func (s *UserService) UpdateUser(user *model.User) (*model.User, error) {
	if user == nil || user.ID == "" {
		return nil, ErrInvalidUser
	}

	// Validate update
	if user.Email != "" {
		if !isValidEmail(user.Email) {
			return nil, fmt.Errorf("invalid email format: %s", user.Email)
		}
		
		// Check for email conflicts
		existing, _ := s.userRepo.FindByEmail(user.Email)
		if existing != nil && existing.ID != user.ID {
			return nil, ErrUserExists
		}
	}

	if user.Age < 0 || user.Age > 150 {
		return nil, fmt.Errorf("invalid age: %d", user.Age)
	}

	// Update timestamp
	user.UpdatedAt = time.Now().Unix()

	// Save changes
	updatedUser, err := s.userRepo.Update(user)
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	s.auditLog.LogUserUpdated(user.ID, user.Email)

	return updatedUser, nil
}

// DeleteUser soft deletes a user (medium complexity)
func (s *UserService) DeleteUser(id string) error {
	if id == "" {
		return ErrInvalidUser
	}

	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return err
	}

	if user == nil {
		return ErrUserNotFound
	}

	// Perform soft delete
	user.Status = model.StatusDeleted
	user.DeletedAt = time.Now().Unix()
	user.UpdatedAt = time.Now().Unix()

	_, err = s.userRepo.Update(user)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	// Send notification
	if err := s.sendDeletionNotification(user); err != nil {
		s.auditLog.LogError("Failed to send deletion notification", err)
	}

	s.auditLog.LogUserDeleted(user.ID, user.Email)

	return nil
}

// SearchUsers performs complex user search with multiple filters (high complexity)
func (s *UserService) SearchUsers(filters model.UserSearchFilters, limit, offset int, sortBy, sortOrder string) ([]*model.User, int, error) {
	// Validate pagination parameters
	if limit <= 0 || limit > 1000 {
		return nil, 0, fmt.Errorf("invalid limit: %d", limit)
	}

	if offset < 0 {
		return nil, 0, fmt.Errorf("invalid offset: %d", offset)
	}

	// Validate sort parameters
	validSortFields := map[string]bool{
		"name": true, "email": true, "age": true, 
		"created_at": true, "updated_at": true,
	}
	
	if !validSortFields[sortBy] {
		return nil, 0, fmt.Errorf("invalid sort field: %s", sortBy)
	}

	if sortOrder != "asc" && sortOrder != "desc" {
		return nil, 0, fmt.Errorf("invalid sort order: %s", sortOrder)
	}

	// Build search query
	query := repository.SearchQuery{
		Limit:     limit,
		Offset:    offset,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}

	// Apply filters with validation
	if filters.Name != nil {
		name := strings.TrimSpace(*filters.Name)
		if len(name) > 0 {
			query.NameFilter = &name
		}
	}

	if filters.Email != nil {
		email := strings.TrimSpace(*filters.Email)
		if len(email) > 0 {
			if !isValidEmail(email) {
				return nil, 0, fmt.Errorf("invalid email filter: %s", email)
			}
			query.EmailFilter = &email
		}
	}

	if filters.Status != nil {
		status := *filters.Status
		switch status {
		case "active", "inactive", "suspended", "deleted":
			query.StatusFilter = &status
		default:
			return nil, 0, fmt.Errorf("invalid status filter: %s", status)
		}
	}

	// Age range validation and application
	if filters.MinAge != nil && filters.MaxAge != nil {
		if *filters.MinAge > *filters.MaxAge {
			return nil, 0, fmt.Errorf("min_age cannot be greater than max_age")
		}
	}

	if filters.MinAge != nil {
		minAge := *filters.MinAge
		if minAge < 0 || minAge > 150 {
			return nil, 0, fmt.Errorf("invalid min_age: %d", minAge)
		}
		query.MinAge = &minAge
	}

	if filters.MaxAge != nil {
		maxAge := *filters.MaxAge
		if maxAge < 0 || maxAge > 150 {
			return nil, 0, fmt.Errorf("invalid max_age: %d", maxAge)
		}
		query.MaxAge = &maxAge
	}

	// Execute search
	users, err := s.userRepo.Search(query)
	if err != nil {
		return nil, 0, fmt.Errorf("search failed: %w", err)
	}

	// Get total count for pagination
	total, err := s.userRepo.CountSearch(query)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get total count: %w", err)
	}

	// Log search activity
	s.auditLog.LogUserSearch(filters, len(users))

	return users, total, nil
}

// ProcessUserBatch processes multiple users in a batch (very high complexity)
func (s *UserService) ProcessUserBatch(requests []*model.BatchUserRequest) (*model.BatchResult, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("empty batch")
	}

	if len(requests) > 1000 {
		return nil, fmt.Errorf("batch too large: %d (max 1000)", len(requests))
	}

	result := &model.BatchResult{
		Total:     len(requests),
		Processed: 0,
		Errors:    []model.BatchError{},
		Results:   []model.BatchItem{},
	}

	// Process each request
	for i, req := range requests {
		var batchItem model.BatchItem
		batchItem.Index = i
		batchItem.RequestID = req.ID

		// Determine operation type and process accordingly
		switch req.Operation {
		case "create":
			if req.CreateData == nil {
				batchItem.Error = "missing create data"
				result.Errors = append(result.Errors, model.BatchError{
					Index:   i,
					Message: "missing create data",
				})
			} else {
				user, err := s.CreateUser(req.CreateData)
				if err != nil {
					batchItem.Error = err.Error()
					result.Errors = append(result.Errors, model.BatchError{
						Index:   i,
						Message: err.Error(),
					})
				} else {
					batchItem.UserID = user.ID
					batchItem.Success = true
					result.Processed++
				}
			}

		case "update":
			if req.UpdateData == nil {
				batchItem.Error = "missing update data"
				result.Errors = append(result.Errors, model.BatchError{
					Index:   i,
					Message: "missing update data",
				})
			} else {
				user, err := s.UpdateUser(req.UpdateData)
				if err != nil {
					batchItem.Error = err.Error()
					result.Errors = append(result.Errors, model.BatchError{
						Index:   i,
						Message: err.Error(),
					})
				} else {
					batchItem.UserID = user.ID
					batchItem.Success = true
					result.Processed++
				}
			}

		case "delete":
			if req.UserID == "" {
				batchItem.Error = "missing user ID"
				result.Errors = append(result.Errors, model.BatchError{
					Index:   i,
					Message: "missing user ID",
				})
			} else {
				err := s.DeleteUser(req.UserID)
				if err != nil {
					batchItem.Error = err.Error()
					result.Errors = append(result.Errors, model.BatchError{
						Index:   i,
						Message: err.Error(),
					})
				} else {
					batchItem.UserID = req.UserID
					batchItem.Success = true
					result.Processed++
				}
			}

		case "activate":
			if req.UserID == "" {
				batchItem.Error = "missing user ID"
				result.Errors = append(result.Errors, model.BatchError{
					Index:   i,
					Message: "missing user ID",
				})
			} else {
				err := s.activateUser(req.UserID)
				if err != nil {
					batchItem.Error = err.Error()
					result.Errors = append(result.Errors, model.BatchError{
						Index:   i,
						Message: err.Error(),
					})
				} else {
					batchItem.UserID = req.UserID
					batchItem.Success = true
					result.Processed++
				}
			}

		case "deactivate":
			if req.UserID == "" {
				batchItem.Error = "missing user ID"
				result.Errors = append(result.Errors, model.BatchError{
					Index:   i,
					Message: "missing user ID",
				})
			} else {
				err := s.deactivateUser(req.UserID)
				if err != nil {
					batchItem.Error = err.Error()
					result.Errors = append(result.Errors, model.BatchError{
						Index:   i,
						Message: err.Error(),
					})
				} else {
					batchItem.UserID = req.UserID
					batchItem.Success = true
					result.Processed++
				}
			}

		default:
			batchItem.Error = fmt.Sprintf("unknown operation: %s", req.Operation)
			result.Errors = append(result.Errors, model.BatchError{
				Index:   i,
				Message: fmt.Sprintf("unknown operation: %s", req.Operation),
			})
		}

		result.Results = append(result.Results, batchItem)
	}

	// Calculate success rate
	result.SuccessRate = float64(result.Processed) / float64(result.Total) * 100

	// Log batch processing result
	s.auditLog.LogBatchProcessed(result.Total, result.Processed, len(result.Errors))

	return result, nil
}

// Private helper methods (should have low to medium complexity)

func (s *UserService) validateCreateRequest(req *model.CreateUserRequest) error {
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}

	if req.Email == "" {
		return fmt.Errorf("email is required")
	}

	if !isValidEmail(req.Email) {
		return fmt.Errorf("invalid email format: %s", req.Email)
	}

	if req.Age < 0 || req.Age > 150 {
		return fmt.Errorf("invalid age: %d", req.Age)
	}

	return nil
}

func (s *UserService) sendWelcomeEmail(user *model.User) error {
	return s.emailService.SendWelcome(user.Email, user.Name)
}

func (s *UserService) sendDeletionNotification(user *model.User) error {
	return s.emailService.SendDeletionNotification(user.Email, user.Name)
}

func (s *UserService) activateUser(userID string) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return err
	}

	if user == nil {
		return ErrUserNotFound
	}

	user.Status = model.StatusActive
	user.ActivatedAt = time.Now().Unix()
	user.UpdatedAt = time.Now().Unix()

	_, err = s.userRepo.Update(user)
	return err
}

func (s *UserService) deactivateUser(userID string) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return err
	}

	if user == nil {
		return ErrUserNotFound
	}

	user.Status = model.StatusInactive
	user.DeactivatedAt = time.Now().Unix()
	user.UpdatedAt = time.Now().Unix()

	_, err = s.userRepo.Update(user)
	return err
}

// Utility functions (low complexity)
func isValidEmail(email string) bool {
	return strings.Contains(email, "@") && strings.Contains(email, ".")
}

func generateID() string {
	return fmt.Sprintf("user_%d", time.Now().UnixNano())
}

// Mock external services
type EmailService struct{}

func (e *EmailService) SendWelcome(email, name string) error {
	return nil // Mock implementation
}

func (e *EmailService) SendDeletionNotification(email, name string) error {
	return nil // Mock implementation
}

type AuditLogger struct{}

func (a *AuditLogger) LogUserCreated(userID, email string) {}
func (a *AuditLogger) LogUserUpdated(userID, email string) {}
func (a *AuditLogger) LogUserDeleted(userID, email string) {}
func (a *AuditLogger) LogUserSearch(filters model.UserSearchFilters, resultCount int) {}
func (a *AuditLogger) LogBatchProcessed(total, processed, errors int) {}
func (a *AuditLogger) LogError(message string, err error) {}