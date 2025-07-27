package handlers

import (
	"net/http"
	"os"
	"resumeai/services"
	"resumeai/utils"
	"time"

	"github.com/gin-gonic/gin"
)

type ResumeRequest struct {
	Position   string   `json:"position"`
	Experience string   `json:"experience"` // ✅ 已变为自由文本
	Education  string   `json:"education"`  // ✅ 新增字段
	Skills     []string `json:"skills"`
}

func GenerateResume(c *gin.Context) {
	var req ResumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// ✅ 使用自由文本构造 prompt
	prompt := services.BuildResumePrompt(req.Experience, req.Education, req.Skills)

	resumeContent, err := services.CallGeminiWithAPIKey(prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 保存 Word 文件到 static/
	saveDir := "./static"
	os.MkdirAll(saveDir, os.ModePerm)

	filename := "resume_" + time.Now().Format("20060102150405") + ".docx"
	filepath := saveDir + "/" + filename

	err = utils.GenerateWordFile(resumeContent, filepath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Resume generated successfully.",
		"filePath": "/static/" + filename,
	})
}
