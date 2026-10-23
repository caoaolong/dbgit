package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Body 统一 API 响应体。
type Body struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// OK 返回成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{
		Code:    0,
		Message: "ok",
		Data:    data,
	})
}

// Fail 返回业务失败响应（HTTP 仍为 200，用 code 区分）。
func Fail(c *gin.Context, code int, message string) {
	c.JSON(http.StatusOK, Body{
		Code:    code,
		Message: message,
	})
}

// Abort 返回 HTTP 错误并中止后续处理。
func Abort(c *gin.Context, httpStatus int, code int, message string) {
	c.AbortWithStatusJSON(httpStatus, Body{
		Code:    code,
		Message: message,
	})
}
