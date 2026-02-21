package models

// AppError is a structured application error with HTTP status code.
type AppError struct {
	Code    string `json:"error"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
	Status  int    `json:"-"`
}

func (e *AppError) Error() string { return e.Message }

// Error constructors.
var (
	ErrNotFound = func(msg string) *AppError {
		return &AppError{Code: "NOT_FOUND", Message: msg, Status: 404}
	}
	ErrBadRequest = func(msg string) *AppError {
		return &AppError{Code: "BAD_REQUEST", Message: msg, Status: 400}
	}
	ErrUnauthorized = &AppError{Code: "UNAUTHORIZED", Message: "authentication required", Status: 401}
	ErrInternal     = func(msg string) *AppError {
		return &AppError{Code: "INTERNAL", Message: msg, Status: 500}
	}
	ErrConflict = func(msg string) *AppError {
		return &AppError{Code: "CONFLICT", Message: msg, Status: 409}
	}
)
