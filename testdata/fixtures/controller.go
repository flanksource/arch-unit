package controller

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"example.com/app/model"
	"example.com/app/service"
)

// UserController handles user-related HTTP requests
type UserController struct {
	userService *service.UserService
}

// NewUserController creates a new UserController
func NewUserController(userService *service.UserService) *UserController {
	return &UserController{
		userService: userService,
	}
}

// GetUser retrieves a user by ID (simple method - low complexity)
func (c *UserController) GetUser(ctx *gin.Context) {
	id := ctx.Param("id")
	if id == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "missing user id"})
		return
	}

	user, err := c.userService.GetUserByID(id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	ctx.JSON(http.StatusOK, user)
}

// CreateUser creates a new user (medium complexity)
func (c *UserController) CreateUser(ctx *gin.Context) {
	var userReq model.CreateUserRequest
	if err := ctx.ShouldBindJSON(&userReq); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validation logic increases complexity
	if userReq.Email == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "email is required"})
		return
	}

	if userReq.Age < 0 || userReq.Age > 150 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid age"})
		return
	}

	user, err := c.userService.CreateUser(&userReq)
	if err != nil {
		if err == service.ErrUserExists {
			ctx.JSON(http.StatusConflict, gin.H{"error": "user already exists"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	ctx.JSON(http.StatusCreated, user)
}

// UpdateUser updates an existing user (high complexity method)
func (c *UserController) UpdateUser(ctx *gin.Context) {
	id := ctx.Param("id")
	var updateReq model.UpdateUserRequest
	
	if err := ctx.ShouldBindJSON(&updateReq); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Complex validation and update logic
	existingUser, err := c.userService.GetUserByID(id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// Nested conditionals increase complexity
	if updateReq.Email != "" {
		if !isValidEmail(updateReq.Email) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid email format"})
			return
		}
		
		// Check for email conflicts
		if updateReq.Email != existingUser.Email {
			conflictUser, err := c.userService.GetUserByEmail(updateReq.Email)
			if err == nil && conflictUser != nil {
				ctx.JSON(http.StatusConflict, gin.H{"error": "email already in use"})
				return
			}
		}
		existingUser.Email = updateReq.Email
	}

	if updateReq.Name != "" {
		existingUser.Name = updateReq.Name
	}

	if updateReq.Age > 0 {
		if updateReq.Age < 13 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "user must be at least 13 years old"})
			return
		}
		if updateReq.Age > 150 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid age"})
			return
		}
		existingUser.Age = updateReq.Age
	}

	// Multiple switch cases add complexity
	switch updateReq.Status {
	case "active":
		existingUser.Status = model.StatusActive
		existingUser.ActivatedAt = timeNow()
	case "inactive":
		existingUser.Status = model.StatusInactive
		existingUser.DeactivatedAt = timeNow()
	case "suspended":
		existingUser.Status = model.StatusSuspended
		existingUser.SuspendedAt = timeNow()
	case "":
		// No status update
	default:
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	updatedUser, err := c.userService.UpdateUser(existingUser)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}

	ctx.JSON(http.StatusOK, updatedUser)
}

// SearchUsers searches for users with complex filtering (very high complexity)
func (c *UserController) SearchUsers(ctx *gin.Context) {
	var filters model.UserSearchFilters
	
	// Query parameter parsing with multiple conditions
	if name := ctx.Query("name"); name != "" {
		filters.Name = &name
	}
	
	if email := ctx.Query("email"); email != "" {
		if !isValidEmail(email) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid email format"})
			return
		}
		filters.Email = &email
	}

	// Age range filtering
	if minAgeStr := ctx.Query("min_age"); minAgeStr != "" {
		minAge, err := parseInt(minAgeStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid min_age"})
			return
		}
		if minAge < 0 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "min_age cannot be negative"})
			return
		}
		filters.MinAge = &minAge
	}

	if maxAgeStr := ctx.Query("max_age"); maxAgeStr != "" {
		maxAge, err := parseInt(maxAgeStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid max_age"})
			return
		}
		if maxAge > 150 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "max_age cannot exceed 150"})
			return
		}
		if filters.MinAge != nil && maxAge < *filters.MinAge {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "max_age cannot be less than min_age"})
			return
		}
		filters.MaxAge = &maxAge
	}

	// Status filtering with validation
	if status := ctx.Query("status"); status != "" {
		switch status {
		case "active", "inactive", "suspended":
			filters.Status = &status
		default:
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid status filter"})
			return
		}
	}

	// Pagination parameters
	limit := 10 // default
	if limitStr := ctx.Query("limit"); limitStr != "" {
		parsedLimit, err := parseInt(limitStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		if parsedLimit <= 0 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "limit must be positive"})
			return
		}
		if parsedLimit > 100 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "limit cannot exceed 100"})
			return
		}
		limit = parsedLimit
	}

	offset := 0 // default
	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		parsedOffset, err := parseInt(offsetStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset"})
			return
		}
		if parsedOffset < 0 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "offset cannot be negative"})
			return
		}
		offset = parsedOffset
	}

	// Sorting parameters
	sortBy := "created_at" // default
	if sort := ctx.Query("sort"); sort != "" {
		switch sort {
		case "name", "email", "age", "created_at", "updated_at":
			sortBy = sort
		default:
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid sort field"})
			return
		}
	}

	sortOrder := "desc" // default
	if order := ctx.Query("order"); order != "" {
		switch order {
		case "asc", "desc":
			sortOrder = order
		default:
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid sort order"})
			return
		}
	}

	// Execute search with all the complex filtering
	users, total, err := c.userService.SearchUsers(filters, limit, offset, sortBy, sortOrder)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "search failed"})
		return
	}

	response := gin.H{
		"users":  users,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}

	ctx.JSON(http.StatusOK, response)
}

// Helper functions (should have low complexity)
func isValidEmail(email string) bool {
	// Simplified email validation
	return len(email) > 0 && containsChar(email, '@') && containsChar(email, '.')
}

func containsChar(s string, char rune) bool {
	for _, c := range s {
		if c == char {
			return true
		}
	}
	return false
}

func parseInt(s string) (int, error) {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result, nil
}

func timeNow() int64 {
	return 1640995200 // Mock timestamp
}