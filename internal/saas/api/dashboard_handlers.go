package api

import (
	"net/http"
	"strconv"

	"quantsaas/internal/saas/auth"
	"quantsaas/internal/saas/dashboard"

	"github.com/gin-gonic/gin"
)

func handleDashboard(service *dashboard.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := auth.CurrentUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		if service == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dashboard service is unavailable"})
			return
		}

		payload, err := service.LoadDashboard(c.Request.Context(), user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, payload)
	}
}

func handleEquitySnapshots(service *dashboard.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := auth.CurrentUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		if service == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dashboard service is unavailable"})
			return
		}

		instanceID, err := strconv.ParseUint(c.Query("instance_id"), 10, 64)
		if err != nil || instanceID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instance_id"})
			return
		}

		rangeDays := 30
		if raw := c.Query("range_days"); raw != "" {
			parsed, parseErr := strconv.Atoi(raw)
			if parseErr != nil || parsed <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid range_days"})
				return
			}
			rangeDays = parsed
		}

		payload, err := service.LoadEquitySnapshots(c.Request.Context(), user.ID, uint(instanceID), rangeDays)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, payload)
	}
}

func handleSystemStatus(service *dashboard.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := auth.CurrentUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		if service == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dashboard service is unavailable"})
			return
		}

		payload, err := service.LoadSystemStatus(c.Request.Context(), user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, payload)
	}
}
