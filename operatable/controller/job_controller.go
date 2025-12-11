package controller

import (
	"net/http"
	"operatable/cerror"
	"operatable/dto"
	"operatable/service"

	"github.com/gin-gonic/gin"
)

type JobController struct {
	jobService *service.JobService
}

func NewJobControllerV1(
	router *gin.RouterGroup,
	jobService *service.JobService,
) *JobController {
	c := &JobController{
		jobService: jobService,
	}

	router = router.Group("/jobs/v1")
	router.GET("", c.FindAll)
	router.POST("", c.Create)
	router.DELETE("", c.Delete)

	return c
}

func (t *JobController) filterServiceError(c *gin.Context, err error) {
	cerr, ok := err.(*cerror.CError)
	if !ok {
		c.AbortWithStatusJSON(
			http.StatusInternalServerError,
			cerror.
				New5(http.StatusInternalServerError).
				WithErrorLogContext(c.Request.Context(), err),
		)
		return
	}

	c.AbortWithStatusJSON(cerr.Status, cerr)
}

// @Summary Get job
// @Description Retrieve the current job
// @Tags Job
// @Accept json
// @Produce json
// @Success 200 {object} dto.FindJobResp
// @Router /api/jobs/v1 [get]
func (t *JobController) FindAll(c *gin.Context) {
	res, err := t.jobService.Find(c.Request.Context())
	if err != nil {
		t.filterServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, res)
}

// @Summary Create a job
// @Description Create a new job
// @Tags Job
// @Accept json
// @Produce json
// @Param job body dto.CreateJobDTO true "Job to create"
// @Success 201 {object} dto.FindJobResp
// @Router /api/jobs/v1 [post]
func (t *JobController) Create(c *gin.Context) {
	var body dto.CreateJobDTO
	if err := c.ShouldBindJSON(&body); err != nil {
		c.AbortWithStatusJSON(
			http.StatusBadRequest,
			cerror.New4(http.StatusBadRequest, err.Error()),
		)
		return
	}

	res, err := t.jobService.Create(c.Request.Context(), &body)
	if err != nil {
		t.filterServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, res)
}

// @Summary Delete job
// @Description Delete the current job
// @Tags Job
// @Accept json
// @Produce json
// @Success 204
// @Router /api/jobs/v1 [delete]
func (t *JobController) Delete(c *gin.Context) {
	err := t.jobService.Delete(c.Request.Context())
	if err != nil {
		t.filterServiceError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}
