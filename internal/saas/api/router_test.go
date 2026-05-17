package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"quantsaas/internal/saas/config"

	"github.com/gin-gonic/gin"
)

func TestRequireInstanceWriteRoleRejectsLab(t *testing.T) {
	t.Parallel()

	router := gin.New()
	router.Use(requireInstanceWriteRole(config.Config{AppRole: config.RoleLab}))
	router.POST("/instances", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/instances", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for lab role, got %d", rec.Code)
	}
}

func TestRequireInstanceWriteRoleAllowsSaaSAndDev(t *testing.T) {
	t.Parallel()

	for _, role := range []string{config.RoleSaaS, config.RoleDev} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()

			router := gin.New()
			router.Use(requireInstanceWriteRole(config.Config{AppRole: role}))
			router.POST("/instances", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodPost, "/instances", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("expected 204 for role %s, got %d", role, rec.Code)
			}
		})
	}
}
