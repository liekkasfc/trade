package auth

import (
	"strings"

	"quantsaas/internal/saas/store"

	"github.com/gin-gonic/gin"
)

const currentUserKey = "auth.current_user"

func RequireAuth(service *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(401, gin.H{"error": "missing bearer token"})
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		claims, err := service.ParseToken(token)
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "invalid token"})
			return
		}

		user, err := service.LoadUser(c.Request.Context(), claims.UserID)
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "user not found"})
			return
		}

		c.Set(currentUserKey, user)
		c.Next()
	}
}

func CurrentUser(c *gin.Context) (*store.User, bool) {
	raw, ok := c.Get(currentUserKey)
	if !ok {
		return nil, false
	}
	user, ok := raw.(*store.User)
	return user, ok
}
