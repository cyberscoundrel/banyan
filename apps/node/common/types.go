// Package common provides shared types and data structures used across the Banyan node application.
// These types are used for HTTP responses, internal communication, and data exchange between
// different components of the node.
package common

import (
	"time"
)

// Response represents a standard HTTP response structure used for API endpoints.
// It contains metadata about the request and response including the response message,
// timestamp, HTTP method, request path, and any associated headers.
type Response struct {
	Message   string              `json:"message"`
	Timestamp time.Time           `json:"timestamp"`
	Method    string              `json:"method"`
	Path      string              `json:"path"`
	Headers   map[string][]string `json:"headers"`
}
