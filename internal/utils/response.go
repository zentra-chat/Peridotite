package utils

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details any    `json:"details,omitempty"`
}

type SuccessResponse struct {
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}

type PaginatedResponse struct {
	Data       any   `json:"data"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"pageSize"`
	TotalPages int   `json:"totalPages"`
}

// RespondJSON writes a JSON response
func RespondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// RespondError writes an error response
func RespondError(w http.ResponseWriter, status int, message string) {
	RespondJSON(w, status, ErrorResponse{Error: message})
}

// RespondErrorWithCode writes an error response with an error code
func RespondErrorWithCode(w http.ResponseWriter, status int, code, message string) {
	RespondJSON(w, status, ErrorResponse{Error: message, Code: code})
}

// RespondValidationError writes a validation error response
func RespondValidationError(w http.ResponseWriter, details any) {
	RespondJSON(w, http.StatusBadRequest, ErrorResponse{
		Error:   "Validation error",
		Code:    "VALIDATION_ERROR",
		Details: details,
	})
}

// RespondSuccess writes a success response
func RespondSuccess(w http.ResponseWriter, data any) {
	RespondJSON(w, http.StatusOK, SuccessResponse{Data: data})
}

// RespondCreated writes a created response
func RespondCreated(w http.ResponseWriter, data any) {
	RespondJSON(w, http.StatusCreated, SuccessResponse{Data: data})
}

// RespondNoContent writes a 204 response
func RespondNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// RespondPaginated writes a paginated response
func RespondPaginated(w http.ResponseWriter, data any, total int64, page, pageSize int) {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	RespondJSON(w, http.StatusOK, PaginatedResponse{
		Data:       data,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

// DecodeJSON decodes a JSON request body
func DecodeJSON(r *http.Request, v any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

// GetQueryInt extracts an integer query parameter with a default value
func GetQueryInt(r *http.Request, key string, defaultValue int) int {
	val := r.URL.Query().Get(key)
	if val == "" {
		return defaultValue
	}
	intVal, err := strconv.Atoi(val)
	if err != nil {
		return defaultValue
	}
	return intVal
}

// GetQueryInt64 extracts an int64 query parameter with a default value
func GetQueryInt64(r *http.Request, key string, defaultValue int64) int64 {
	val := r.URL.Query().Get(key)
	if val == "" {
		return defaultValue
	}
	intVal, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return defaultValue
	}
	return intVal
}

// GetQueryBool extracts a boolean query parameter with a default value
func GetQueryBool(r *http.Request, key string, defaultValue bool) bool {
	val := r.URL.Query().Get(key)
	if val == "" {
		return defaultValue
	}
	boolVal, err := strconv.ParseBool(val)
	if err != nil {
		return defaultValue
	}
	return boolVal
}

// GetQueryString extracts a string query parameter with a default value
func GetQueryString(r *http.Request, key string, defaultValue string) string {
	val := r.URL.Query().Get(key)
	if val == "" {
		return defaultValue
	}
	return val
}
