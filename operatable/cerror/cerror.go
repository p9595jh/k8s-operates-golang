package cerror

import (
	"context"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

var _ error = (*CError)(nil)

type CError struct {
	Status    int     `json:"status"`
	Message   string  `json:"message"`
	Timestamp string  `json:"timestamp"`
	Method    *string `json:"method,omitempty"`
	Path      *string `json:"path,omitempty"`
}

func New4(status int, message string) *CError {
	return &CError{
		Status:    status,
		Message:   message,
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

func New5(status int) *CError {
	return &CError{
		Status:    status,
		Message:   http.StatusText(status),
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

func (c *CError) Error() string {
	return c.Message
}

func (c *CError) WithRequest(r *http.Request) *CError {
	if c.Status < http.StatusInternalServerError {
		c.Method = &r.Method
		c.Path = &r.URL.Path
	}
	return c
}

func (c *CError) WithErrorLogContext(ctx context.Context, err error) *CError {
	log.Error().Ctx(ctx).Err(err).Send()
	return c
}
