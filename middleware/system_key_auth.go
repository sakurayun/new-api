package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// SystemKeyAuth 系统 Key 认证中间件
// 从请求头 Authorization: Bearer sk-sys-xxx 提取 Key 并验证
func SystemKeyAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHeader := c.Request.Header.Get("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "未提供系统 Key",
			})
			c.Abort()
			return
		}

		// 提取 Bearer token
		key := strings.TrimPrefix(authHeader, "Bearer ")
		if key == authHeader {
			// 没有 Bearer 前缀，直接使用
			key = authHeader
		}

		// 验证系统 Key
		systemKey, err := model.ValidateSystemKey(key)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": err.Error(),
			})
			c.Abort()
			return
		}

		// 写入 context
		c.Set("system_key_id", systemKey.Id)
		c.Set("system_key_name", systemKey.Name)
		c.Next()
	}
}
