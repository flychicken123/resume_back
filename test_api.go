package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func main() {
	// Test registration
	fmt.Println("Testing user registration...")

	registrationData := map[string]interface{}{
		"email":    "test@example.com",
		"password": "password123",
	}

	jsonData, _ := json.Marshal(registrationData)

	resp, err := http.Post("http://localhost:8081/api/auth/register",
		"application/json", bytes.NewBuffer(jsonData))

	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n", string(body))

	// Test login
	fmt.Println("\nTesting user login...")

	loginData := map[string]interface{}{
		"email":    "test@example.com",
		"password": "password123",
	}

	jsonData, _ = json.Marshal(loginData)

	resp, err = http.Post("http://localhost:8081/api/auth/login",
		"application/json", bytes.NewBuffer(jsonData))

	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n", string(body))
}
