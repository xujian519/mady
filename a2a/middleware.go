package a2a

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Auth middleware
// ---------------------------------------------------------------------------

func withAuth(next http.Handler, cfg AuthConfig) http.Handler {
	if cfg.APIKey == "" && cfg.BearerToken == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authenticated := false

		if cfg.APIKey != "" {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-API-Key")), []byte(cfg.APIKey)) == 1 {
				authenticated = true
			}
		}

		if !authenticated && cfg.BearerToken != "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") && subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, "Bearer ")), []byte(cfg.BearerToken)) == 1 {
				authenticated = true
			}
		}

		if !authenticated {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   &JSONRPCError{Code: JSONRPCInvalidRequest, Message: "unauthorized"},
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// CORS middleware
// ---------------------------------------------------------------------------

func withCORS(next http.Handler, cfg CORSConfig) http.Handler {
	if len(cfg.AllowOrigins) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := false
		// Note: a bare "*" entry only grants a match when credentials are not
		// allowed. Reflecting an arbitrary Origin while also sending
		// Access-Control-Allow-Credentials: true would let any site make
		// credentialed requests, defeating CORS protection.
		for _, o := range cfg.AllowOrigins {
			if o == origin || (o == "*" && !cfg.AllowCredentials) {
				allowed = true
				break
			}
		}
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			if cfg.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if len(cfg.AllowMethods) > 0 {
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ", "))
			}
			if len(cfg.AllowHeaders) > 0 {
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ", "))
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
