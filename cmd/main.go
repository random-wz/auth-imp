package main

import (
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"syscall"

	"github.com/google/uuid"
	"github.com/idp-service/internal/api"
	"github.com/idp-service/internal/auth"
	"github.com/idp-service/internal/directory"
	"github.com/idp-service/internal/model"
	"github.com/idp-service/internal/service"
	"github.com/idp-service/internal/uds"
	"github.com/idp-service/internal/user"
)

const (
	bootstrapAdminUser     = "admin"
	bootstrapAdminPassword = "Admin@123456"
	bootstrapAdminEmail    = "admin@example.com"
)

func main() {
	// 设置资源限制
	setResourceLimits()
	// 初始化依赖
	userStore := user.NewMemoryStore()
	syncStore := user.NewMemorySyncJobStore()
	orgStore := directory.NewMemoryOrgStore()
	groupStore := directory.NewMemoryGroupStore()
	authSvc := auth.NewService(auth.Config{
		JWTSecret: "dev-secret-key-change-in-production",
	})

	// 注入初始 admin 用户（bootstrap）
	bootstrapAdmin(userStore, authSvc)

	// 初始化服务层
	userSvc := service.NewUserService(userStore, syncStore, authSvc)
	dirSvc := service.NewDirectoryService(orgStore, groupStore, userStore)

	// 初始化 UDS 服务端
	registry := uds.NewHandlerRegistry(userSvc, dirSvc)
	udsServer := uds.NewServer(uds.Config{
		SocketPath: "/tmp/idp-uds.sock",
		MaxConns:   100,
		Registry:   registry,
	})
	if err := udsServer.Start(); err != nil {
		log.Fatalf("Failed to start UDS server: %v", err)
	}

	// 初始化 REST API
	handler := api.NewHandler(userSvc, authSvc)
	dirHandler := api.NewDirectoryHandler(dirSvc)
	router := api.SetupRouter(handler, dirHandler, authSvc)

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Println("REST API server starting on :8080")
		if err := router.Run(":8080"); err != nil {
			log.Printf("REST API server error: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down...")
	udsServer.Stop()
}

func bootstrapAdmin(store user.Store, authSvc *auth.Service) {
	hash, err := authSvc.HashPassword(bootstrapAdminPassword)
	if err != nil {
		log.Fatalf("Failed to hash bootstrap password: %v", err)
	}
	admin := &model.User{
		ID:           uuid.New().String(),
		Username:     bootstrapAdminUser,
		Email:        bootstrapAdminEmail,
		PasswordHash: hash,
		DisplayName:  "System Administrator",
		Status:       model.UserStatusActive,
	}
	if err := store.Create(admin); err != nil {
		log.Fatalf("Failed to create bootstrap admin: %v", err)
	}
	log.Printf("[Bootstrap] Admin user created: username=%s password=%s",
		bootstrapAdminUser, bootstrapAdminPassword)
}

func setResourceLimits() {
	// CPU 限制（GOMAXPROCS）
	cpuLimit := getEnvInt("CPU_LIMIT", 1)
	runtime.GOMAXPROCS(cpuLimit)
	log.Printf("[Resource] CPU limit: %d core(s)", cpuLimit)

	// 内存限制（GOMEMLIMIT）
	memLimit := getEnvInt("MEM_LIMIT_MB", 50)
	debug.SetMemoryLimit(int64(memLimit) * 1024 * 1024)
	log.Printf("[Resource] Memory limit: %d MB", memLimit)
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}
