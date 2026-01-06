package handlers

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"resumeai/services"
	"resumeai/utils"
)

var voiceAgent *services.VoiceAgent

// SetVoiceAgent injects the LangChain-powered voice transcription agent.
func SetVoiceAgent(agent *services.VoiceAgent) {
	voiceAgent = agent
}

// TranscribeAudio handles audio transcription requests.
func TranscribeAudio(c *gin.Context) {
	if voiceAgent == nil {
		utils.InternalServerError(c, "Voice agent is not configured", nil)
		return
	}

	// Get the audio file from the request
	file, header, err := c.Request.FormFile("audio")
	if err != nil {
		utils.BadRequestError(c, "No audio file provided", err)
		return
	}
	defer file.Close()

	// Read audio data
	audioData, err := io.ReadAll(file)
	if err != nil {
		utils.InternalServerError(c, "Failed to read audio file", err)
		return
	}

	// Get mime type from header or form field
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = c.PostForm("mimeType")
	}
	if mimeType == "" {
		mimeType = "audio/webm"
	}

	// Transcribe audio
	result, err := voiceAgent.TranscribeAudio(c.Request.Context(), audioData, mimeType)
	if err != nil {
		utils.InternalServerError(c, "Failed to transcribe audio", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result,
	})
}
