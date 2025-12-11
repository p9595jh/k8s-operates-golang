package main

import (
	"operatable/config"
	"operatable/controller"
	_ "operatable/docs"
	"operatable/service"
	"time"

	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func main() {

	// set timezone
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.FixedZone("Asia/Seoul", 9*60*60)
	}
	time.Local = loc

	// service
	jobService := service.NewJobService()

	// controller
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(func(ctx *gin.Context) {
		startAt := time.Now()
		ctx.Next()
		log.Info().
			Str("method", ctx.Request.Method).
			Str("path", ctx.Request.URL.Path).
			Str("client_ip", ctx.ClientIP()).
			Dur("latency", time.Since(startAt)).
			Int("status", ctx.Writer.Status()).
			Send()
	})
	setupSwagger(router)

	routerGroup := router.Group("/api")
	controller.NewHealthControllerV1(routerGroup)
	controller.NewJobControllerV1(routerGroup, jobService)

	log.Info().Msgf("server is running at :%s", config.Get().App.Port)
	if err := router.Run(":" + config.Get().App.Port); err != nil {
		log.Fatal().Err(err).Send()
	}
}

func setupSwagger(r *gin.Engine) {
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/swagger/index.html")
	})

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
