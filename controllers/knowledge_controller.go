package controllers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"resumeai/services"
)

type KnowledgeController struct {
	svc *services.KnowledgeService
}

func NewKnowledgeController(svc *services.KnowledgeService) *KnowledgeController {
	return &KnowledgeController{svc: svc}
}

func (kc *KnowledgeController) List(c *gin.Context) {
	entries, err := kc.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries, "total": len(entries)})
}

func (kc *KnowledgeController) Create(c *gin.Context) {
	var req struct {
		Title    string `json:"title" binding:"required"`
		Category string `json:"category" binding:"required"`
		Content  string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := kc.svc.AddEntry(c.Request.Context(), req.Title, req.Category, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "embedded": true})
}

func (kc *KnowledgeController) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Title    string `json:"title" binding:"required"`
		Category string `json:"category" binding:"required"`
		Content  string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := kc.svc.UpdateEntry(c.Request.Context(), id, req.Title, req.Category, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "re_embedded": true})
}

func (kc *KnowledgeController) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := kc.svc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (kc *KnowledgeController) BackfillEmbeddings(c *gin.Context) {
	processed, err := kc.svc.BackfillEmbeddings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"processed": processed})
}
