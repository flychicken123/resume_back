package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "http://localhost:8081/api"

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token   string      `json:"token"`
	User    interface{} `json:"user"` // Can be string or object
	UserID  int         `json:"user_id,omitempty"`
	Success bool        `json:"success"`
	Message string      `json:"message"`
}

func main() {
	// First, create a test user or login
	fmt.Println("=== Testing Resume Generation Limits ===\n")

	// Register/Login as a free user with timestamp to avoid conflicts
	email := fmt.Sprintf("testfree_%d@example.com", time.Now().Unix())
	token, userID := loginOrRegister(email, "password123")
	if token == "" {
		fmt.Println("Failed to authenticate")
		return
	}

	fmt.Printf("Authenticated as user ID: %d\n", userID)

	// Test 1: Try to generate first resume (should succeed for free user)
	fmt.Println("\nTest 1: First resume generation (should succeed)")
	if testResumeGeneration(token, 1) {
		fmt.Println("✓ First resume generated successfully")
	} else {
		fmt.Println("✗ First resume generation failed")
	}

	// Test 2: Try to generate second resume immediately (should fail for free user)
	fmt.Println("\nTest 2: Second resume generation immediately (should fail)")
	if !testResumeGeneration(token, 2) {
		fmt.Println("✓ Second resume blocked correctly (limit enforced)")
	} else {
		fmt.Println("✗ Second resume should have been blocked")
	}

	// Test 3: Check the limit endpoint
	fmt.Println("\nTest 3: Checking limit status")
	checkLimitStatus(token)
}

func loginOrRegister(email, password string) (string, int) {
	// Try login first
	loginReq := LoginRequest{Email: email, Password: password}
	reqBody, _ := json.Marshal(loginReq)

	resp, err := http.Post(baseURL+"/auth/login", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return "", 0
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var loginResp LoginResponse
		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &loginResp)
		// For login, user might be an object with ID
		if userMap, ok := loginResp.User.(map[string]interface{}); ok {
			if id, ok := userMap["id"].(float64); ok {
				return loginResp.Token, int(id)
			}
		}
		return loginResp.Token, 1 // Default user ID if not found
	}

	// If login failed, try to register
	fmt.Println("Login failed, trying to register...")

	registerReq := map[string]string{
		"email":    email,
		"password": password,
		"name":     "Test Free User",
	}
	reqBody, _ = json.Marshal(registerReq)

	resp, err = http.Post(baseURL+"/auth/register", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return "", 0
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		var loginResp LoginResponse
		body, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(body, &loginResp); err != nil {
			fmt.Printf("Failed to parse register response: %v\n", err)
			fmt.Printf("Response body: %s\n", string(body))
			return "", 0
		}
		// For register, just return token with a default user ID
		// The actual user ID would need to be extracted from JWT if needed
		return loginResp.Token, 1
	}

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Registration failed with status %d: %s\n", resp.StatusCode, string(body))
	return "", 0
}

func testResumeGeneration(token string, attemptNum int) bool {
	resumeData := map[string]interface{}{
		"name":       "Test User",
		"email":      "test@example.com",
		"phone":      "555-1234",
		"position":   "Software Engineer",
		"summary":    "Experienced software engineer",
		"experience": "Software Engineer at Tech Co (2020-2024): Built and maintained web applications",
		"education":  "BS Computer Science, Tech University, 2020",
		"skills":     []string{"JavaScript", "Python", "Go"},
		"format":     "html",
	}

	reqBody, _ := json.Marshal(resumeData)

	req, _ := http.NewRequest("POST", baseURL+"/resume/generate", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 200 {
		fmt.Printf("  Attempt %d: Success (Status: %d)\n", attemptNum, resp.StatusCode)
		return true
	} else if resp.StatusCode == 403 {
		var errorResp map[string]interface{}
		json.Unmarshal(body, &errorResp)
		fmt.Printf("  Attempt %d: Blocked - %v\n", attemptNum, errorResp["error"])
		if limitReached, ok := errorResp["limitReached"].(bool); ok && limitReached {
			fmt.Printf("  Limit details: %s plan, %v/%v resumes\n",
				errorResp["plan"], errorResp["remaining"], errorResp["limit"])
		}
		return false
	} else {
		fmt.Printf("  Attempt %d: Failed (Status: %d) - %s\n", attemptNum, resp.StatusCode, string(body))
		return false
	}
}

func checkLimitStatus(token string) {
	req, _ := http.NewRequest("GET", baseURL+"/subscription/check-limit", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 200 {
		var limitResp map[string]interface{}
		json.Unmarshal(body, &limitResp)
		fmt.Printf("  Current limit status:\n")
		fmt.Printf("    Can generate: %v\n", limitResp["can_generate"])
		fmt.Printf("    Remaining: %v\n", limitResp["remaining"])
		fmt.Printf("    Plan: %v\n", limitResp["plan"])
		if resetDate, ok := limitResp["reset_date"].(string); ok && resetDate != "" {
			fmt.Printf("    Reset date: %v\n", resetDate)
		}
	} else {
		fmt.Printf("  Failed to check limit (Status: %d)\n", resp.StatusCode)
	}
}