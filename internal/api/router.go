package api

import (
	"net/http/pprof"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/idp-service/internal/auth"
)

// SetupRouter 配置 Gin 路由
func SetupRouter(h *Handler, dh *DirectoryHandler, authSvc *auth.Service) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// pprof 性能分析端点（可选）
	if os.Getenv("ENABLE_PPROF") == "true" {
		debug := r.Group("/debug/pprof")
		{
			debug.GET("/", gin.WrapF(pprof.Index))
			debug.GET("/cmdline", gin.WrapF(pprof.Cmdline))
			debug.GET("/profile", gin.WrapF(pprof.Profile))
			debug.GET("/symbol", gin.WrapF(pprof.Symbol))
			debug.GET("/trace", gin.WrapF(pprof.Trace))
			debug.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))
			debug.GET("/heap", gin.WrapH(pprof.Handler("heap")))
			debug.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
			debug.GET("/block", gin.WrapH(pprof.Handler("block")))
			debug.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))
		}
	}

	v1 := r.Group("/api/v1")
	{
		// 认证路由（无需 JWT）
		authGroup := v1.Group("/auth")
		{
			authGroup.POST("/login", h.Login)
		}

		// 登出路由（需要 JWT）
		authProtected := v1.Group("/auth")
		authProtected.Use(JWTMiddleware(authSvc))
		{
			authProtected.POST("/logout", h.Logout)
			authProtected.POST("/online", h.Online)
		}

		// 用户路由（需要 JWT）
		users := v1.Group("/users")
		users.Use(JWTMiddleware(authSvc))
		{
			users.POST("", h.CreateUser)
			users.GET("", h.ListUsers)
			users.GET("/:id", h.GetUser)
			users.PUT("/:id", h.UpdateUser)
			users.DELETE("/:id", h.DeleteUser)
			// 用户所属组织与群组
			users.GET("/:id/orgs", dh.GetUserOrgs)
			users.GET("/:id/groups", dh.GetUserGroups)
		}

		// 组织路由（需要 JWT）
		orgs := v1.Group("/orgs")
		orgs.Use(JWTMiddleware(authSvc))
		{
			orgs.POST("", dh.CreateOrg)
			orgs.GET("", dh.ListOrgs)
			orgs.GET("/:id", dh.GetOrg)
			orgs.PUT("/:id", dh.UpdateOrg)
			orgs.DELETE("/:id", dh.DeleteOrg)
			orgs.GET("/:id/children", dh.ListOrgChildren)
			orgs.POST("/:id/members", dh.AddOrgMember)
			orgs.GET("/:id/members", dh.ListOrgMembers)
			orgs.DELETE("/:id/members/:userId", dh.RemoveOrgMember)
		}

		// 群组路由（需要 JWT）
		groups := v1.Group("/groups")
		groups.Use(JWTMiddleware(authSvc))
		{
			groups.POST("", dh.CreateGroup)
			groups.GET("", dh.ListGroups)
			groups.GET("/:id", dh.GetGroup)
			groups.PUT("/:id", dh.UpdateGroup)
			groups.DELETE("/:id", dh.DeleteGroup)
			groups.POST("/:id/members", dh.AddGroupMember)
			groups.GET("/:id/members", dh.ListGroupMembers)
			groups.DELETE("/:id/members/:userId", dh.RemoveGroupMember)
		}

		// 同步任务路由（需要 JWT）
		sync := v1.Group("/sync")
		sync.Use(JWTMiddleware(authSvc))
		{
			sync.POST("/jobs", h.CreateSyncJob)
			sync.GET("/jobs/:id", h.GetSyncJob)
		}
	}

	// SCIM 2.0 端点
	scim := r.Group("/scim/v2")
	scim.Use(JWTMiddleware(authSvc))
	{
		scim.GET("/Users", h.ListUsers)
		scim.POST("/Users", h.CreateUser)
		scim.GET("/Users/:id", h.GetUser)
		scim.PUT("/Users/:id", h.UpdateUser)
		scim.DELETE("/Users/:id", h.DeleteUser)
	}

	return r
}
