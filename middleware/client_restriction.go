package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func ClientRestriction() gin.HandlerFunc {
	return func(c *gin.Context) {
		usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
		result := service.CheckClientRestriction(c, usingGroup)
		service.SetClientRestrictionResult(c, result)
		if result.Enabled && !result.Allowed {
			service.AbortForClientRestriction(c, result)
			return
		}
		c.Next()
	}
}
