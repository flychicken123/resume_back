package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"resumeai/services"
)

type GeoController struct {
	service *services.GeoService
}

func NewGeoController(service *services.GeoService) *GeoController {
	return &GeoController{service: service}
}

func (gc *GeoController) GetAnswers(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, gc.service.Feed())
}
