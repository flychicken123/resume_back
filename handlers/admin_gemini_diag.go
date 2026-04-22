package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"resumeai/services"
)

// GeminiDiagHandler makes a single trivial Gemini Flash call and returns the
// full error payload so operators can see exactly which quota/metric is
// being exceeded without needing GCP console access.
func GeminiDiagHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		reply, err := services.CallGeminiFlashWithTemperature("Reply with exactly the word OK.", 0.0)
		elapsedMs := time.Since(start).Milliseconds()

		if err == nil {
			c.JSON(http.StatusOK, gin.H{
				"ok":         true,
				"reply":      reply,
				"latency_ms": elapsedMs,
			})
			return
		}

		// Extract every diagnostic field we can from the error chain.
		payload := gin.H{
			"ok":             false,
			"latency_ms":     elapsedMs,
			"error":          err.Error(),
			"is_rate_limit":  services.IsRateLimitErr(err),
		}

		var apiErr *googleapi.Error
		if errors.As(err, &apiErr) {
			payload["googleapi_code"] = apiErr.Code
			payload["googleapi_msg"] = apiErr.Message
			payload["googleapi_body"] = apiErr.Body
			payload["googleapi_details"] = apiErr.Details
		}

		if grpcStatus, ok := status.FromError(err); ok && grpcStatus != nil {
			payload["grpc_code"] = grpcStatus.Code().String()
			payload["grpc_msg"] = grpcStatus.Message()
			payload["grpc_details"] = grpcStatus.Details()
		}

		if codes.ResourceExhausted == status.Code(err) {
			payload["grpc_resource_exhausted"] = true
		}

		c.JSON(http.StatusOK, payload)
	}
}
