package server

import (
	"net/http"

	"github.com/Autumn-27/artex/agent"
)

// requestLocale accepts only the public locale contract, never arbitrary header text.
func requestLocale(r *http.Request) string {
	return agent.NormalizeLocale(r.Header.Get("X-Artex-Locale"))
}

func requestLocaleOr(r *http.Request, stored string) string {
	if locale := requestLocale(r); locale != "" {
		return locale
	}
	return agent.NormalizeLocale(stored)
}
