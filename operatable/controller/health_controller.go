package controller

import "github.com/gin-gonic/gin"

type HealthController struct{}

func NewHealthControllerV1(router *gin.RouterGroup) *HealthController {
	c := &HealthController{}

	router = router.Group("/health")
	router.GET("", c.Health)

	return c
}

// @Summary Health Check
// @Description Check the health status of the API
// @Tags Health
// @Accept json
// @Produce json
// @Router /api/health [get]
func (h *HealthController) Health(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": "ok",
	})
}
