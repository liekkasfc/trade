package api

import (
	"net/http"
	"strconv"

	"quantsaas/internal/saas/backtests"
	"quantsaas/internal/saas/epoch"

	"github.com/gin-gonic/gin"
)

func handleCreateBacktest(service *backtests.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req backtests.CreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		task, err := service.CreateAndRun(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

func handleGetBacktest(service *backtests.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backtest id"})
			return
		}
		task, err := service.Get(c.Request.Context(), uint(id))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, task)
	}
}

func handleCreateEvolutionTask(service *epoch.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req epoch.CreateTaskRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		task, err := service.CreateAndRunTask(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

func handleListEvolutionTasks(service *epoch.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		tasks, err := service.ListTasks(c.Request.Context(), c.Query("template_id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"tasks": tasks})
	}
}

func handleListEvolutionGenomes(service *epoch.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		records, err := service.ListGenomes(c.Request.Context(), c.Query("template_id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"genomes": records})
	}
}

func handlePromoteEvolutionTask(service *epoch.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid evolution task id"})
			return
		}
		if err := service.Promote(c.Request.Context(), uint(id)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "promoted"})
	}
}

func handleGetChampionGenome(service *epoch.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		record, err := service.GetChampion(c.Request.Context(), c.Query("template_id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"champion": record})
	}
}

func handleListChallengerGenomes(service *epoch.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		records, err := service.ListChallengers(c.Request.Context(), c.Query("template_id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"genomes": records})
	}
}
