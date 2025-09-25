package response

import (
	"encoding/json"
	"net/http"
	"time"
)

type Response struct {
	Success   bool        `json:"success"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Error     *ErrorInfo  `json:"error,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

type ErrorInfo struct {
	Code    string      `json:"code,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

func Success(w http.ResponseWriter, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := Response{
		Success:   true,
		Message:   message,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(response)
}

func Error(w http.ResponseWriter, statusCode int, message string, details interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := Response{
		Success: false,
		Message: message,
		Error: &ErrorInfo{
			Code:    http.StatusText(statusCode),
			Details: details,
		},
		Timestamp: time.Now().Unix(),
	}

	json.NewEncoder(w).Encode(response)
}

func JSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}
