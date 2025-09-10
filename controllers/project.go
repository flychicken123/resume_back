package controllers

import (
	"net/http"
	"resumeai/models"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ProjectController struct {
	projectModel *models.ProjectModel
}

func NewProjectController(projectModel *models.ProjectModel) *ProjectController {
	return &ProjectController{
		projectModel: projectModel,
	}
}

// GetProjectsByResumeID gets all projects for a resume
func (pc *ProjectController) GetProjectsByResumeID(c *gin.Context) {
	resumeIDStr := c.Param("resumeId")
	resumeID, err := strconv.Atoi(resumeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resume ID"})
		return
	}

	projects, err := pc.projectModel.GetByResumeID(resumeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch projects"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

// CreateProject creates a new project
func (pc *ProjectController) CreateProject(c *gin.Context) {
	var project models.Project
	if err := c.ShouldBindJSON(&project); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := pc.projectModel.Create(&project); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"project": project})
}

// UpdateProject updates an existing project
func (pc *ProjectController) UpdateProject(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	var project models.Project
	if err := c.ShouldBindJSON(&project); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	project.ID = id
	if err := pc.projectModel.Update(&project); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update project"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"project": project})
}

// DeleteProject deletes a project
func (pc *ProjectController) DeleteProject(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	if err := pc.projectModel.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete project"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Project deleted successfully"})
}