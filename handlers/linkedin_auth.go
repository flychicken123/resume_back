package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"resumeai/models"
	"resumeai/services"
	"strings"

	"github.com/gin-gonic/gin"
)

type LinkedInTokenRequest struct {
	Code        string `json:"code" binding:"required"`
	RedirectURI string `json:"redirect_uri" binding:"required"`
}

type LinkedInTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

type LinkedInProfile struct {
	ID        string `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"emailAddress"`
}

func GetLinkedInAuthURL(c *gin.Context) {
	clientID := os.Getenv("LINKEDIN_CLIENT_ID")
	if clientID == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "LinkedIn client ID not configured"})
		return
	}
	
	redirectURI := c.Query("redirect_uri")
	if redirectURI == "" {
		redirectURI = "http://localhost:3000/linkedin-callback"
	}
	
	state := c.Query("state")
	if state == "" {
		state = "random_state_string"
	}
	
	authURL := fmt.Sprintf(
		"https://www.linkedin.com/oauth/v2/authorization?response_type=code&client_id=%s&redirect_uri=%s&state=%s&scope=openid%%20profile%%20email",
		clientID,
		url.QueryEscape(redirectURI),
		state,
	)
	
	c.JSON(http.StatusOK, gin.H{
		"auth_url": authURL,
	})
}

func ExchangeLinkedInToken(userModel *models.UserModel, jwtService *services.JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LinkedInTokenRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		
		clientID := os.Getenv("LINKEDIN_CLIENT_ID")
		if clientID == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "LinkedIn client ID not configured"})
			return
		}
		
		clientSecret := os.Getenv("LINKEDIN_CLIENT_SECRET")
		if clientSecret == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "LinkedIn client secret not configured"})
			return
		}
		
		// Exchange authorization code for access token
		tokenURL := "https://www.linkedin.com/oauth/v2/accessToken"
		
		data := url.Values{}
		data.Set("grant_type", "authorization_code")
		data.Set("code", req.Code)
		data.Set("redirect_uri", req.RedirectURI)
		data.Set("client_id", clientID)
		data.Set("client_secret", clientSecret)
		
		resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to exchange token"})
			return
		}
		defer resp.Body.Close()
		
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
			return
		}
		
		if resp.StatusCode != http.StatusOK {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get access token", "details": string(body)})
			return
		}
		
		var tokenResp LinkedInTokenResponse
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse token response"})
			return
		}
		
		// Get user profile from LinkedIn
		profile, err := getLinkedInProfile(tokenResp.AccessToken)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get LinkedIn profile"})
			return
		}
		
		// Check if user exists
		user, err := userModel.GetByEmail(profile.Email)
		if err != nil {
			// Create new user
			user, err = userModel.CreateWithProvider(
				profile.Email,
				fmt.Sprintf("%s %s", profile.FirstName, profile.LastName),
				"", // No password for OAuth users
				"linkedin",
				profile.ID,
				"", // Profile picture URL if available
			)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
				return
			}
		}
		
		// Generate JWT token
		token, err := jwtService.GenerateToken(user.ID, user.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
			return
		}
		
		c.JSON(http.StatusOK, gin.H{
			"message":       "LinkedIn login successful",
			"token":         token,
			"user":          user,
			"linkedin_token": tokenResp.AccessToken,
		})
	}
}

func getLinkedInProfile(accessToken string) (*LinkedInProfile, error) {
	profileURL := "https://api.linkedin.com/v2/me"
	emailURL := "https://api.linkedin.com/v2/emailAddress?q=members&projection=(elements*(handle~))"
	
	// Get basic profile
	req, err := http.NewRequest("GET", profileURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+accessToken)
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	var profileData map[string]interface{}
	if err := json.Unmarshal(body, &profileData); err != nil {
		return nil, err
	}
	
	// Get email address
	emailReq, err := http.NewRequest("GET", emailURL, nil)
	if err != nil {
		return nil, err
	}
	emailReq.Header.Add("Authorization", "Bearer "+accessToken)
	
	emailResp, err := client.Do(emailReq)
	if err != nil {
		return nil, err
	}
	defer emailResp.Body.Close()
	
	emailBody, err := io.ReadAll(emailResp.Body)
	if err != nil {
		return nil, err
	}
	
	var emailData map[string]interface{}
	if err := json.Unmarshal(emailBody, &emailData); err != nil {
		return nil, err
	}
	
	// Extract email from response
	email := ""
	if elements, ok := emailData["elements"].([]interface{}); ok && len(elements) > 0 {
		if element, ok := elements[0].(map[string]interface{}); ok {
			if handle, ok := element["handle~"].(map[string]interface{}); ok {
				if emailAddress, ok := handle["emailAddress"].(string); ok {
					email = emailAddress
				}
			}
		}
	}
	
	// Extract name from profile
	firstName := ""
	lastName := ""
	id := ""
	
	if firstNameData, ok := profileData["localizedFirstName"].(string); ok {
		firstName = firstNameData
	}
	if lastNameData, ok := profileData["localizedLastName"].(string); ok {
		lastName = lastNameData
	}
	if idData, ok := profileData["id"].(string); ok {
		id = idData
	}
	
	return &LinkedInProfile{
		ID:        id,
		FirstName: firstName,
		LastName:  lastName,
		Email:     email,
	}, nil
}

func FetchLinkedInResume(c *gin.Context) {
	token := c.GetHeader("X-LinkedIn-Token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "LinkedIn token required"})
		return
	}
	
	// Fetch profile data
	profileURL := "https://api.linkedin.com/v2/me?projection=(id,firstName,lastName,profilePicture(displayImage~:playableStreams))"
	
	req, err := http.NewRequest("GET", profileURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}
	req.Header.Add("Authorization", "Bearer "+token)
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch profile"})
		return
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response"})
		return
	}
	
	// Fetch positions/experience
	positionsURL := "https://api.linkedin.com/v2/positions?q=members&projection=(elements*(company,title,description,startDate,endDate))"
	
	posReq, err := http.NewRequest("GET", positionsURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create positions request"})
		return
	}
	posReq.Header.Add("Authorization", "Bearer "+token)
	
	posResp, err := client.Do(posReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch positions"})
		return
	}
	defer posResp.Body.Close()
	
	posBody, err := io.ReadAll(posResp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read positions response"})
		return
	}
	
	// Parse and format the data
	var profileData map[string]interface{}
	var positionsData map[string]interface{}
	
	json.Unmarshal(body, &profileData)
	json.Unmarshal(posBody, &positionsData)
	
	// Format experience data
	experiences := []map[string]interface{}{}
	if elements, ok := positionsData["elements"].([]interface{}); ok {
		for _, elem := range elements {
			if pos, ok := elem.(map[string]interface{}); ok {
				exp := map[string]interface{}{
					"company":     pos["company"],
					"role":        pos["title"],
					"description": pos["description"],
					"startDate":   pos["startDate"],
					"endDate":     pos["endDate"],
				}
				experiences = append(experiences, exp)
			}
		}
	}
	
	c.JSON(http.StatusOK, gin.H{
		"profile":     profileData,
		"experiences": experiences,
	})
}