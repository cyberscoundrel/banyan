package common

import (
	"time"
)

// Response represents a standard HTTP response structure
type Response struct {
	Message   string              `json:"message"`
	Timestamp time.Time           `json:"timestamp"`
	Method    string              `json:"method"`
	Path      string              `json:"path"`
	Headers   map[string][]string `json:"headers"`
}
