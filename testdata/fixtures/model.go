package model

// User represents a user in the system
type User struct {
	ID            string `json:"id" db:"id"`
	Name          string `json:"name" db:"name"`
	Email         string `json:"email" db:"email"`
	Age           int    `json:"age" db:"age"`
	Status        string `json:"status" db:"status"`
	CreatedAt     int64  `json:"created_at" db:"created_at"`
	UpdatedAt     int64  `json:"updated_at" db:"updated_at"`
	ActivatedAt   int64  `json:"activated_at,omitempty" db:"activated_at"`
	DeactivatedAt int64  `json:"deactivated_at,omitempty" db:"deactivated_at"`
	SuspendedAt   int64  `json:"suspended_at,omitempty" db:"suspended_at"`
	DeletedAt     int64  `json:"deleted_at,omitempty" db:"deleted_at"`
}

// User status constants
const (
	StatusActive    = "active"
	StatusInactive  = "inactive"
	StatusSuspended = "suspended"
	StatusDeleted   = "deleted"
)

// CreateUserRequest represents a request to create a new user
type CreateUserRequest struct {
	Name  string `json:"name" binding:"required"`
	Email string `json:"email" binding:"required,email"`
	Age   int    `json:"age" binding:"required,min=0,max=150"`
}

// UpdateUserRequest represents a request to update a user
type UpdateUserRequest struct {
	Name   string `json:"name,omitempty"`
	Email  string `json:"email,omitempty"`
	Age    int    `json:"age,omitempty"`
	Status string `json:"status,omitempty"`
}

// UserSearchFilters represents filters for user search
type UserSearchFilters struct {
	Name   *string `json:"name,omitempty"`
	Email  *string `json:"email,omitempty"`
	Status *string `json:"status,omitempty"`
	MinAge *int    `json:"min_age,omitempty"`
	MaxAge *int    `json:"max_age,omitempty"`
}

// UserStatistics represents user statistics
type UserStatistics struct {
	TotalUsers     int     `json:"total_users"`
	ActiveUsers    int     `json:"active_users"`
	InactiveUsers  int     `json:"inactive_users"`
	SuspendedUsers int     `json:"suspended_users"`
	AverageAge     float64 `json:"average_age"`
	Under18        int     `json:"under_18"`
	Age18to24      int     `json:"age_18_to_24"`
	Age25to34      int     `json:"age_25_to_34"`
	Age35to49      int     `json:"age_35_to_49"`
	Age50Plus      int     `json:"age_50_plus"`
}

// BatchUserRequest represents a single request in a batch operation
type BatchUserRequest struct {
	ID         string             `json:"id"`
	Operation  string             `json:"operation"` // create, update, delete, activate, deactivate
	UserID     string             `json:"user_id,omitempty"`
	CreateData *CreateUserRequest `json:"create_data,omitempty"`
	UpdateData *User              `json:"update_data,omitempty"`
}

// BatchResult represents the result of a batch operation
type BatchResult struct {
	Total       int          `json:"total"`
	Processed   int          `json:"processed"`
	SuccessRate float64      `json:"success_rate"`
	Errors      []BatchError `json:"errors"`
	Results     []BatchItem  `json:"results"`
}

// BatchError represents an error in a batch operation
type BatchError struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// BatchItem represents the result of a single item in a batch operation
type BatchItem struct {
	Index     int    `json:"index"`
	RequestID string `json:"request_id"`
	UserID    string `json:"user_id,omitempty"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// Profile represents extended user profile information
type Profile struct {
	UserID      string   `json:"user_id" db:"user_id"`
	FirstName   string   `json:"first_name" db:"first_name"`
	LastName    string   `json:"last_name" db:"last_name"`
	PhoneNumber string   `json:"phone_number" db:"phone_number"`
	Address     *Address `json:"address,omitempty"`
	Preferences *UserPreferences `json:"preferences,omitempty"`
	CreatedAt   int64    `json:"created_at" db:"created_at"`
	UpdatedAt   int64    `json:"updated_at" db:"updated_at"`
}

// Address represents a user's address
type Address struct {
	Street     string `json:"street" db:"street"`
	City       string `json:"city" db:"city"`
	State      string `json:"state" db:"state"`
	PostalCode string `json:"postal_code" db:"postal_code"`
	Country    string `json:"country" db:"country"`
}

// UserPreferences represents user preferences and settings
type UserPreferences struct {
	Language         string            `json:"language" db:"language"`
	Timezone         string            `json:"timezone" db:"timezone"`
	EmailNotifications bool            `json:"email_notifications" db:"email_notifications"`
	SMSNotifications   bool            `json:"sms_notifications" db:"sms_notifications"`
	Theme            string            `json:"theme" db:"theme"`
	NotificationTypes map[string]bool  `json:"notification_types,omitempty"`
}

// Role represents a user role
type Role struct {
	ID          string       `json:"id" db:"id"`
	Name        string       `json:"name" db:"name"`
	Description string       `json:"description" db:"description"`
	Permissions []Permission `json:"permissions,omitempty"`
	CreatedAt   int64        `json:"created_at" db:"created_at"`
	UpdatedAt   int64        `json:"updated_at" db:"updated_at"`
}

// Permission represents a permission that can be granted to roles
type Permission struct {
	ID          string `json:"id" db:"id"`
	Name        string `json:"name" db:"name"`
	Resource    string `json:"resource" db:"resource"`
	Action      string `json:"action" db:"action"`
	Description string `json:"description" db:"description"`
}

// UserRole represents the association between users and roles
type UserRole struct {
	UserID    string `json:"user_id" db:"user_id"`
	RoleID    string `json:"role_id" db:"role_id"`
	GrantedAt int64  `json:"granted_at" db:"granted_at"`
	GrantedBy string `json:"granted_by" db:"granted_by"`
	ExpiresAt int64  `json:"expires_at,omitempty" db:"expires_at"`
}

// Session represents a user session
type Session struct {
	ID        string `json:"id" db:"id"`
	UserID    string `json:"user_id" db:"user_id"`
	Token     string `json:"token" db:"token"`
	IPAddress string `json:"ip_address" db:"ip_address"`
	UserAgent string `json:"user_agent" db:"user_agent"`
	CreatedAt int64  `json:"created_at" db:"created_at"`
	ExpiresAt int64  `json:"expires_at" db:"expires_at"`
	LastUsedAt int64 `json:"last_used_at" db:"last_used_at"`
}

// AuditLog represents an audit log entry
type AuditLog struct {
	ID        string                 `json:"id" db:"id"`
	UserID    string                 `json:"user_id" db:"user_id"`
	Action    string                 `json:"action" db:"action"`
	Resource  string                 `json:"resource" db:"resource"`
	Details   map[string]interface{} `json:"details,omitempty"`
	IPAddress string                 `json:"ip_address" db:"ip_address"`
	UserAgent string                 `json:"user_agent" db:"user_agent"`
	CreatedAt int64                  `json:"created_at" db:"created_at"`
}

// LoginAttempt represents a login attempt
type LoginAttempt struct {
	ID          string `json:"id" db:"id"`
	Email       string `json:"email" db:"email"`
	IPAddress   string `json:"ip_address" db:"ip_address"`
	UserAgent   string `json:"user_agent" db:"user_agent"`
	Success     bool   `json:"success" db:"success"`
	FailureReason string `json:"failure_reason,omitempty" db:"failure_reason"`
	AttemptedAt   int64  `json:"attempted_at" db:"attempted_at"`
}

// Notification represents a notification
type Notification struct {
	ID        string                 `json:"id" db:"id"`
	UserID    string                 `json:"user_id" db:"user_id"`
	Type      string                 `json:"type" db:"type"`
	Title     string                 `json:"title" db:"title"`
	Message   string                 `json:"message" db:"message"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Read      bool                   `json:"read" db:"read"`
	ReadAt    int64                  `json:"read_at,omitempty" db:"read_at"`
	CreatedAt int64                  `json:"created_at" db:"created_at"`
}

// Device represents a user device
type Device struct {
	ID           string `json:"id" db:"id"`
	UserID       string `json:"user_id" db:"user_id"`
	DeviceType   string `json:"device_type" db:"device_type"`
	DeviceName   string `json:"device_name" db:"device_name"`
	PushToken    string `json:"push_token,omitempty" db:"push_token"`
	LastSeenAt   int64  `json:"last_seen_at" db:"last_seen_at"`
	RegisteredAt int64  `json:"registered_at" db:"registered_at"`
	Active       bool   `json:"active" db:"active"`
}

// APIKey represents an API key for programmatic access
type APIKey struct {
	ID          string   `json:"id" db:"id"`
	UserID      string   `json:"user_id" db:"user_id"`
	Name        string   `json:"name" db:"name"`
	KeyHash     string   `json:"-" db:"key_hash"` // Never expose the hash
	Permissions []string `json:"permissions" db:"permissions"`
	LastUsedAt  int64    `json:"last_used_at,omitempty" db:"last_used_at"`
	ExpiresAt   int64    `json:"expires_at,omitempty" db:"expires_at"`
	CreatedAt   int64    `json:"created_at" db:"created_at"`
	Active      bool     `json:"active" db:"active"`
}

// Integration represents third-party integrations
type Integration struct {
	ID           string                 `json:"id" db:"id"`
	UserID       string                 `json:"user_id" db:"user_id"`
	Provider     string                 `json:"provider" db:"provider"`
	ExternalID   string                 `json:"external_id" db:"external_id"`
	AccessToken  string                 `json:"-" db:"access_token"`  // Encrypted
	RefreshToken string                 `json:"-" db:"refresh_token"` // Encrypted
	ExpiresAt    int64                  `json:"expires_at,omitempty" db:"expires_at"`
	Scopes       []string               `json:"scopes" db:"scopes"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt    int64                  `json:"created_at" db:"created_at"`
	UpdatedAt    int64                  `json:"updated_at" db:"updated_at"`
	Active       bool                   `json:"active" db:"active"`
}