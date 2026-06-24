// Package router wires HTTP routes to controllers and middleware.
package router

import (
	"time"

	"github.com/modex/agt-vault/controller"
	"github.com/modex/agt-vault/middleware"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

// Setup builds the Gin engine with all routes and middleware.
//
// Route groups:
//
//	/api/auth/*       — public (login) + self-service (logout, password, self)
//	/api/supplier/*   — SupplierAuth: channel upload/list/update/delete/resync
//	/api/admin/*      — AdminAuth: platforms, users, grants, audit
func Setup(sessionSecret string) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.SecurityHeaders())

	// Session store. Cookie is HTTP-only, SameSite=Lax, Secure in production
	// (set via the Secure option once TLS is terminated in front).
	store := cookie.NewStore([]byte(sessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		HttpOnly: true,
		MaxAge:   86400,
		SameSite: 3, // http.SameSiteStrictMode
	})
	r.Use(sessions.Sessions("agt_session", store))

	api := r.Group("/api")

	// --- auth ---
	auth := api.Group("/auth")
	{
		// Strict rate limit on login to slow credential stuffing.
		auth.POST("/login", middleware.RateLimit(10, time.Minute), controller.Login)
		auth.POST("/logout", controller.Logout)
		auth.GET("/self", middleware.SupplierAuth(), controller.Self)
		auth.POST("/change-password", middleware.SupplierAuth(), controller.ChangePassword)
	}

	// --- supplier ---
	supplier := api.Group("/supplier", middleware.SupplierAuth())
	{
		supplier.GET("/channels", controller.ListChannels)
		supplier.POST("/channels", controller.CreateChannel)
		supplier.PUT("/channels/:id", controller.UpdateChannel)
		supplier.DELETE("/channels/:id", controller.DeleteChannel)
		supplier.POST("/channels/:id/resync", controller.ResyncChannel)
		// Suppliers can see which platforms they are authorized for (no secrets).
		supplier.GET("/platforms", controller.ListMyPlatforms)
	}

	// --- admin ---
	admin := api.Group("/admin", middleware.AdminAuth())
	{
		admin.GET("/platforms", controller.ListPlatforms)
		admin.POST("/platforms", controller.CreatePlatform)
		admin.PUT("/platforms/:id", controller.UpdatePlatform)
		admin.DELETE("/platforms/:id", controller.DeletePlatform)

		admin.GET("/users", controller.ListUsers)
		admin.POST("/users", controller.CreateUser)
		admin.PUT("/users/:id", controller.UpdateUser)
		admin.POST("/users/:id/reset-password", controller.ResetUserPassword)
		admin.DELETE("/users/:id", controller.DeleteUser)

		admin.GET("/grants", controller.ListGrants)
		admin.POST("/grants", controller.UpsertGrant)
		admin.DELETE("/grants/:id", controller.DeleteGrant)

		admin.GET("/audit-logs", controller.ListAuditLogs)
	}

	// Health check (no auth).
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	return r
}
