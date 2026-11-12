package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	computeclient "github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/compute_client"
)

// apiError is sent as the CONTRACT error format: {error: {code, message, details}} (B13).
type apiError struct {
	status  int
	code    string
	message string
	details map[string]any
}

func (e *apiError) Error() string { return e.code + ": " + e.message }

func badRequest(field, message string) *apiError {
	return &apiError{http.StatusBadRequest, "invalid_request", message, map[string]any{"field": field}}
}

func notFound(message string) *apiError {
	return &apiError{status: http.StatusNotFound, code: "not_found", message: message}
}

var errInternal = &apiError{status: http.StatusInternalServerError, code: "internal", message: "Something went wrong. Please try again."}

// fail aborts the request with e.
func fail(c *gin.Context, e *apiError) {
	details := e.details
	if details == nil {
		details = map[string]any{}
	}
	c.AbortWithStatusJSON(e.status, computeclient.ErrorBody{Error: computeclient.ErrorDetail{
		Code: e.code, Message: e.message, Details: details,
	}})
}

// fromCompute maps a compute call failure to a public error.
func fromCompute(err error) *apiError {
	var ce *computeclient.Error
	switch {
	case errors.As(err, &ce) && ce.Status == http.StatusNotFound:
		return notFound(ce.Message)
	case errors.Is(err, computeclient.ErrUnavailable):
		return &apiError{status: http.StatusServiceUnavailable, code: "compute_unavailable",
			message: "The data service is unavailable. Please try again later."}
	case errors.As(err, &ce):
		return &apiError{status: http.StatusBadGateway, code: "compute_error",
			message: "The data service could not answer this request."}
	}
	return errInternal
}
