package utils

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

// StandardResponse represents a standard API response
type StandardResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error"`
	Code    int    `json:"code"`
}

// SuccessResponse sends a successful response
func SuccessResponse(c *gin.Context, statusCode int, message string, data interface{}) {
	c.JSON(statusCode, StandardResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// ErrorResponseWithCode sends an error response with custom status code
func ErrorResponseWithCode(c *gin.Context, statusCode int, message string, err error) {
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}
	
	c.JSON(statusCode, ErrorResponse{
		Success: false,
		Message: message,
		Error:   errorMsg,
		Code:    statusCode,
	})
}

// BadRequestError sends a 400 error response
func BadRequestError(c *gin.Context, message string, err error) {
	ErrorResponseWithCode(c, http.StatusBadRequest, message, err)
}

// InternalServerError sends a 500 error response
func InternalServerError(c *gin.Context, message string, err error) {
	ErrorResponseWithCode(c, http.StatusInternalServerError, message, err)
}

// UnauthorizedError sends a 401 error response
func UnauthorizedError(c *gin.Context, message string) {
	ErrorResponseWithCode(c, http.StatusUnauthorized, message, nil)
}

// NotFoundError sends a 404 error response
func NotFoundError(c *gin.Context, message string) {
	ErrorResponseWithCode(c, http.StatusNotFound, message, nil)
}

// ValidationError sends a validation error response
func ValidationError(c *gin.Context, err error) {
	BadRequestError(c, "Validation failed", err)
}