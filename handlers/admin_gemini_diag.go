package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"resumeai/services"
)

// GeminiDiagHandler makes a single trivial Gemini Flash call via the SDK and
// returns the full error payload.
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

		payload := gin.H{
			"ok":            false,
			"latency_ms":    elapsedMs,
			"error":         err.Error(),
			"is_rate_limit": services.IsRateLimitErr(err),
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

// GeminiRawDiagHandler bypasses the SDK and makes a direct HTTP POST to the
// Gemini Developer API endpoint. Dumps ALL response headers so we can see
// quota-related metadata Google might include that the SDK hides.
func GeminiRawDiagHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		model := c.DefaultQuery("model", "gemini-2.0-flash")
		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": "GEMINI_API_KEY not set"})
			return
		}

		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
		body := map[string]interface{}{
			"contents": []map[string]interface{}{{
				"parts": []map[string]string{{"text": "OK"}},
			}},
			"generationConfig": map[string]interface{}{
				"maxOutputTokens": 5,
				"temperature":     0,
			},
		}
		bodyBytes, _ := json.Marshal(body)

		req, err := http.NewRequestWithContext(c.Request.Context(), "POST", url, nil)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
			return
		}
		req.Body = io.NopCloser(newByteReader(bodyBytes))
		req.ContentLength = int64(len(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		start := time.Now()
		resp, err := client.Do(req)
		elapsedMs := time.Since(start).Milliseconds()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error(), "latency_ms": elapsedMs})
			return
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		headers := map[string]string{}
		for k, v := range resp.Header {
			if len(v) > 0 {
				headers[k] = v[0]
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"ok":            resp.StatusCode == 200,
			"http_status":   resp.StatusCode,
			"latency_ms":    elapsedMs,
			"url_host":      "generativelanguage.googleapis.com",
			"model":         model,
			"response_body": string(respBody),
			"headers":       headers,
		})
	}
}

// GeminiBurstDiagHandler fires N concurrent Gemini calls (default 15, max 30)
// and reports how many succeeded. Helps infer the actual RPM ceiling without
// having to read a quota dashboard.
//
// Interpretation:
//   - If 1-2 succeed and rest fail -> quota near 0 available (free tier ~10 RPM or saturated).
//   - If 10-12 succeed and rest fail -> looks like free tier 10 RPM.
//   - If all 15+ succeed -> paid tier with headroom.
func GeminiBurstDiagHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		count := 15
		if v := c.Query("count"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 30 {
				count = n
			}
		}

		type result struct {
			Index      int    `json:"i"`
			OK         bool   `json:"ok"`
			LatencyMS  int64  `json:"ms"`
			RateLimit  bool   `json:"rl,omitempty"`
			HTTPStatus int    `json:"http,omitempty"`
			Err        string `json:"err,omitempty"`
		}

		start := time.Now()
		results := make([]result, count)

		var wg sync.WaitGroup
		wg.Add(count)
		for i := 0; i < count; i++ {
			i := i
			go func() {
				defer wg.Done()
				callStart := time.Now()
				_, err := services.CallGeminiFlashWithTemperature(fmt.Sprintf("Reply: %d", i), 0.0)
				results[i] = result{
					Index:     i,
					OK:        err == nil,
					LatencyMS: time.Since(callStart).Milliseconds(),
				}
				if err != nil {
					results[i].Err = truncate(err.Error(), 120)
					results[i].RateLimit = services.IsRateLimitErr(err)
					var apiErr *googleapi.Error
					if errors.As(err, &apiErr) {
						results[i].HTTPStatus = apiErr.Code
					}
				}
			}()
		}
		wg.Wait()
		totalMs := time.Since(start).Milliseconds()

		succeeded := 0
		rateLimited := 0
		for _, r := range results {
			if r.OK {
				succeeded++
			}
			if r.RateLimit {
				rateLimited++
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"count":           count,
			"succeeded":       succeeded,
			"rate_limited":    rateLimited,
			"other_errors":    count - succeeded - rateLimited,
			"total_ms":        totalMs,
			"results":         results,
			"interpretation": interpretBurst(count, succeeded, rateLimited),
		})
	}
}

// GeminiMultiModelDiagHandler tries a small call against several Gemini models
// concurrently. Each model has its OWN per-minute quota bucket — if some
// succeed and others fail, we know which generation model is specifically
// saturated vs a project-wide cap.
func GeminiMultiModelDiagHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": "GEMINI_API_KEY not set"})
			return
		}

		models := []string{
			"gemini-2.0-flash",
			"gemini-2.0-flash-lite",
			"gemini-1.5-flash",
			"gemini-1.5-pro",
			"gemini-embedding-001",
		}

		type result struct {
			Model      string `json:"model"`
			OK         bool   `json:"ok"`
			LatencyMS  int64  `json:"ms"`
			HTTPStatus int    `json:"http,omitempty"`
			Snippet    string `json:"snippet,omitempty"`
			Err        string `json:"err,omitempty"`
		}

		results := make([]result, len(models))
		var wg sync.WaitGroup
		wg.Add(len(models))

		for i, m := range models {
			i, m := i, m
			go func() {
				defer wg.Done()
				start := time.Now()
				var url string
				var body map[string]interface{}
				if m == "gemini-embedding-001" {
					url = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:embedContent?key=%s", m, apiKey)
					body = map[string]interface{}{
						"content": map[string]interface{}{
							"parts": []map[string]string{{"text": "hello"}},
						},
					}
				} else {
					url = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", m, apiKey)
					body = map[string]interface{}{
						"contents": []map[string]interface{}{{
							"parts": []map[string]string{{"text": "OK"}},
						}},
						"generationConfig": map[string]interface{}{
							"maxOutputTokens": 5,
							"temperature":     0,
						},
					}
				}
				bodyBytes, _ := json.Marshal(body)
				req, _ := http.NewRequestWithContext(context.Background(), "POST", url, nil)
				req.Body = io.NopCloser(newByteReader(bodyBytes))
				req.ContentLength = int64(len(bodyBytes))
				req.Header.Set("Content-Type", "application/json")

				client := &http.Client{Timeout: 10 * time.Second}
				resp, err := client.Do(req)
				ms := time.Since(start).Milliseconds()
				r := result{Model: m, LatencyMS: ms}
				if err != nil {
					r.Err = truncate(err.Error(), 120)
					results[i] = r
					return
				}
				defer resp.Body.Close()
				r.HTTPStatus = resp.StatusCode
				r.OK = resp.StatusCode == 200
				b, _ := io.ReadAll(resp.Body)
				r.Snippet = truncate(string(b), 180)
				results[i] = r
			}()
		}
		wg.Wait()

		c.JSON(http.StatusOK, gin.H{
			"results": results,
			"hint":    "Models with their own quota pool are labeled separately in GCP. If some succeed and others 429, the failing ones are model-specific saturated. If all 429 equally, it's a project-wide issue (billing/tier).",
		})
	}
}

func interpretBurst(total, ok, rl int) string {
	switch {
	case ok == total:
		return "All calls succeeded. You have plenty of RPM headroom — not saturated."
	case ok == 0 && rl == total:
		return "All calls rate-limited. The RPM quota is at zero available right now — either a very low tier (free = 10 RPM) or something is burning 100% of the quota continuously."
	case ok <= 2 && rl >= total-2:
		return fmt.Sprintf("Only %d/%d succeeded before rate-limit kicked in. Strong indicator of free tier (10 RPM) or near-zero headroom.", ok, total)
	case ok >= 10 && ok <= 13:
		return fmt.Sprintf("%d/%d succeeded before rate-limit. Classic free-tier 10-RPM pattern.", ok, total)
	case ok >= total-3:
		return fmt.Sprintf("%d/%d succeeded. Most requests are getting through — likely paid tier with some headroom.", ok, total)
	default:
		return fmt.Sprintf("%d/%d succeeded, %d rate-limited. Partial saturation — you're at the edge of your quota.", ok, total, rl)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// byteReader is an io.Reader over a byte slice; we use a local impl to avoid
// an import just for bytes.NewReader from a large package.
type byteReader struct {
	data []byte
	pos  int
}

func newByteReader(b []byte) *byteReader { return &byteReader{data: b} }
func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
