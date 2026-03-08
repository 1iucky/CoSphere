package middleware

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowCredentials = true
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"*"}
	// 暴露自定义计费响应头，使浏览器前端可以读取
	config.ExposeHeaders = []string{
		"X-New-Api-Billing-Source",
		"X-New-Api-Billing-Skip-Reason",
	}
	return cors.New(config)
}
