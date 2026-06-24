package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Standard envelope matching the AGT platform contract:
//
//	success: { "success": true,  "message": "", "data": {...} }
//	failure: { "success": false, "message": "reason" }
//
// Keeping this identical to AGT means our own API and any AGT-compatible client
// speak the same shape.

// ApiSuccess writes a 200 success envelope with optional data.
func ApiSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}

// ApiSuccessMsg writes a 200 success envelope carrying a human message and data.
func ApiSuccessMsg(c *gin.Context, message string, data any) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": message,
		"data":    data,
	})
}

// ApiError writes a failure envelope at the given HTTP status.
func ApiError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"message": message,
	})
}

// AbortError writes a failure envelope and stops the middleware chain.
func AbortError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"success": false,
		"message": message,
	})
	c.Abort()
}
