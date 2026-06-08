package errs

import (
	"encoding/json"
	"net/http"
)

type APIError struct {
	Code    int    `json:"-"`
	Message string `json:"error"`
	Detail  string `json:"detail,omitempty"`
}

func (e *APIError) Error() string { return e.Message }

func New(code int, message string) *APIError {
	return &APIError{Code: code, Message: message}
}

func BadRequest(msg string) *APIError  { return New(http.StatusBadRequest, msg) }
func Unauthorized(msg string) *APIError { return New(http.StatusUnauthorized, msg) }
func Forbidden(msg string) *APIError   { return New(http.StatusForbidden, msg) }
func NotFound(msg string) *APIError    { return New(http.StatusNotFound, msg) }
func Internal(msg string) *APIError    { return New(http.StatusInternalServerError, msg) }
func Conflict(msg string) *APIError    { return New(http.StatusConflict, msg) }

func Write(w http.ResponseWriter, err *APIError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Code)
	json.NewEncoder(w).Encode(err) //nolint:errcheck
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
