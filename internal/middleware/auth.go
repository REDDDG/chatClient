package middleware

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// 中间件实现了对id的检验，使用该中间件的路径不再需要对c.get("id")进行检查
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		id := session.Get("id")
		if id == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{})
			return
		}
		uid, ok := id.(int)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{})
		}
		c.Set("id", uid)
		c.Next()
	}
}
