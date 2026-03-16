// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package abort

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

func Abort(c *gin.Context, err error, code int) {
	slog.ErrorContext(c.Request.Context(), "there was a problem", "error", err.Error(), "code", code)
	c.JSON(code, gin.H{"message": err.Error()})
	c.Abort()
}
