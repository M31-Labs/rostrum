// Package actionflow adapts GoSX action responses for progressive enhancement.
package actionflow

import (
	"net/http"
	"strings"

	"m31labs.dev/gosx/action"
)

// Redirect preserves native POST-redirect-GET while returning a readable JSON
// result to managed GoSX forms. A 303 JSON response would otherwise be followed
// by fetch before the navigation runtime can read its redirect field.
func Redirect(ctx *action.Context, target string) {
	if ctx == nil {
		return
	}
	ctx.Redirect(target)
	if wantsJSON(ctx.Request) {
		ctx.SetStatus(http.StatusOK)
	}
}

func wantsJSON(request *http.Request) bool {
	if request == nil {
		return false
	}
	return strings.Contains(request.Header.Get("Accept"), "application/json") ||
		request.Header.Get("X-Requested-With") == "XMLHttpRequest"
}
