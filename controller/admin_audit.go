package controller

import (
	"net/http"
	"strconv"

	"github.com/modex/modex-cloud/common"
	"github.com/modex/modex-cloud/model"

	"github.com/gin-gonic/gin"
)

// ListAuditLogs returns audit entries (admin view), filterable by ?action and
// ?user_id, paginated. Audit details never contain secrets by construction.
func ListAuditLogs(c *gin.Context) {
	action := c.Query("action")
	userId, _ := strconv.Atoi(c.Query("user_id"))
	offset, limit := pageParams(c)

	logs, total, err := model.ListAudit(action, userId, offset, limit)
	if err != nil {
		common.ApiError(c, http.StatusInternalServerError, "failed to list audit logs")
		return
	}
	common.ApiSuccess(c, gin.H{"items": logs, "total": total})
}
