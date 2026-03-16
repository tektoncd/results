// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package pagination

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/pkg/abort"
)

const (
	limitKey        = "limit"
	continueKey     = "continue"
	continueDateKey = "continueDate"
	continueIdKey   = "continueId"
	defaultLimit    = "100"
	maxAllowedLimit = 1000
	minAllowedLimit = 1
)

var ErrBadLimit = errors.New("bad limit parameter")
var ErrBadContinue = errors.New("bad continue token")

// GetValuesFromContext is a helper function for routes to retrieve the
// information needed. This is kept here, so it's close to the function
// that sets these values in the context (Middleware)
func GetValuesFromContext(context *gin.Context) (int, string, string) {
	return context.GetInt(limitKey), context.GetString(continueIdKey), context.GetString(continueDateKey)
}

func CreateToken(uuid int64, date string) string {
	// The date is returned as a quoted string, so remove the quotes
	date = strings.TrimPrefix(date, "\"")
	date = strings.TrimSuffix(date, "\"")
	if date == "" && uuid == 0 {
		return ""
	}
	tokenString := fmt.Sprintf("%d %s", uuid, date)
	return base64.StdEncoding.EncodeToString([]byte(tokenString))
}

// Middleware validates the `limit` and `continue` query parameters
// and populates `limit` and `continueValue` in the context with their
// respective values, so they are retrieved by the endpoints that need it
func Middleware() gin.HandlerFunc {
	return func(context *gin.Context) {
		// We always use a default limit because we don't want to return
		// large collections if users don't remember to specify a limit
		limitString := context.DefaultQuery(limitKey, defaultLimit)
		continueToken := context.Query(continueKey)

		limitInteger, err := strconv.Atoi(limitString)
		if err != nil {
			slog.Error("limit parameter could not be converted", "value", limitString)
			abort.Abort(context, ErrBadLimit, http.StatusBadRequest)
			return
		}
		if limitInteger > maxAllowedLimit {
			slog.Error("limit parameter is bigger than the maximum allowed", "value", limitInteger, "max", maxAllowedLimit)
			abort.Abort(context, ErrBadLimit, http.StatusBadRequest)
			return
		}
		if limitInteger < minAllowedLimit {
			slog.Error("limit parameter is smaller than the maximum allowed", "value", limitInteger, "min", minAllowedLimit)
			abort.Abort(context, ErrBadLimit, http.StatusBadRequest)
			return
		}

		var continueDate string
		var continueId string
		if continueToken != "" {
			continueBytes, err := base64.StdEncoding.DecodeString(continueToken)
			if err != nil {
				slog.Error("could not decode the continue token", "token", continueToken, "error", err.Error())
				abort.Abort(context, ErrBadContinue, http.StatusBadRequest)
				return
			}

			continueParts := strings.Split(string(continueBytes), " ")
			if len(continueParts) != 2 {
				slog.Error("expected two elements on the continue token, received a different amount", "token", string(continueBytes))
				abort.Abort(context, ErrBadContinue, http.StatusBadRequest)
				return
			}

			continueId = continueParts[0]
			// Because the id is an int64 we need to use `ParseInt`
			_, err = strconv.ParseInt(continueId, 10, 64)
			if err != nil {
				slog.Error("id of the continue token is not a valid int64", "id", continueId, "error", err.Error())
				abort.Abort(context, ErrBadContinue, http.StatusBadRequest)
				return
			}

			continueDate = continueParts[1]
			continueTimestamp, err := time.Parse(time.RFC3339, continueDate)
			if err != nil {
				slog.Error("date of the continue token does not match RFC3339", "date", continueDate, "error", err.Error())
				abort.Abort(context, ErrBadContinue, http.StatusBadRequest)
				return
			}

			// We reserialize to avoid SQL injection. There is the possibility the
			// value is a valid date, but in SQL does something else.
			continueDate = continueTimestamp.Format(time.RFC3339)
		}

		// Pass the values using the context, these should be retrieved
		context.Set(limitKey, limitInteger)
		context.Set(continueDateKey, continueDate)
		context.Set(continueIdKey, continueId)
	}
}
