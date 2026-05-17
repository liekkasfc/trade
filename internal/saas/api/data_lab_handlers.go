package api

import (
	"net/http"
	"strconv"

	"quantsaas/internal/saas/datalab"

	"github.com/gin-gonic/gin"
)

func handleSyncDataLab(service *datalab.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if service == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "data lab service is not available"})
			return
		}

		var req datalab.SyncRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		result, err := service.Sync(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "synced", "symbol": result.Symbol, "limit": result.RequestedLimit, "fetched_bars": result.FetchedBars})
	}
}

func handleImportDataLabCSV(service *datalab.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if service == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "data lab service is not available"})
			return
		}

		symbol := c.PostForm("symbol")
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing csv file"})
			return
		}
		handle, err := file.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to open csv file"})
			return
		}
		defer handle.Close()

		result, err := service.ImportCSV(c.Request.Context(), symbol, handle)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func handleGetDataLabCoverage(service *datalab.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if service == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "data lab service is not available"})
			return
		}

		items, err := service.Coverage(c.Request.Context(), c.Query("symbol"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"coverage": items})
	}
}

func handleGetDataLabRecent(service *datalab.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if service == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "data lab service is not available"})
			return
		}

		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "24"))
		bars, err := service.Recent(c.Request.Context(), c.Query("symbol"), limit)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"bars": bars})
	}
}
