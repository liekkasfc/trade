package api

import (
	"net/http"
	"strconv"

	"quantsaas/internal/protocol"
	"quantsaas/internal/saas/auth"
	"quantsaas/internal/saas/instance"

	"github.com/gin-gonic/gin"
)

func handleListStrategies(manager *instance.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		templates, err := manager.ListStrategies(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"strategies": templates})
	}
}

func handleGetStrategy(manager *instance.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		template, err := manager.GetStrategy(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, template)
	}
}

func handleListInstances(manager *instance.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := auth.CurrentUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		instances, err := manager.ListInstances(c.Request.Context(), user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"instances": instances})
	}
}

func handleCreateInstance(manager *instance.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := auth.CurrentUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		var req instance.CreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		created, err := manager.Create(c.Request.Context(), user.ID, req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, created)
	}
}

func handleStartInstance(manager *instance.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		withInstanceID(c, func(instanceID uint, userID uint) {
			if err := manager.Start(c.Request.Context(), userID, instanceID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "running"})
		})
	}
}

func handleStopInstance(manager *instance.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		withInstanceID(c, func(instanceID uint, userID uint) {
			if err := manager.Stop(c.Request.Context(), userID, instanceID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "stopped"})
		})
	}
}

func handleDeleteInstance(manager *instance.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		withInstanceID(c, func(instanceID uint, userID uint) {
			if err := manager.Delete(c.Request.Context(), userID, instanceID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "deleted"})
		})
	}
}

func handleListInstanceLots(manager *instance.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		withInstanceID(c, func(instanceID uint, userID uint) {
			lots, err := manager.ListLots(c.Request.Context(), userID, instanceID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"lots": lots})
		})
	}
}

func handleListInstanceTrades(manager *instance.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		withInstanceID(c, func(instanceID uint, userID uint) {
			trades, err := manager.ListTrades(c.Request.Context(), userID, instanceID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"trades": trades})
		})
	}
}

func handleAgentStatus(hub interface {
	StatusForUser(uint) protocol.AgentStatus
}) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := auth.CurrentUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		if hub == nil {
			c.JSON(http.StatusOK, protocol.AgentStatus{})
			return
		}
		c.JSON(http.StatusOK, hub.StatusForUser(user.ID))
	}
}

func withInstanceID(c *gin.Context, fn func(instanceID uint, userID uint)) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instance id"})
		return
	}
	fn(uint(id), user.ID)
}
