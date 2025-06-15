package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
)

func handleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		callbackURL := url.URL{
			Scheme: "http",
			Host:   "localhost:8080",
			Path:   "/v1/callback",
		}
		query := callbackURL.Query()
		query.Set("code", "abc-123")
		query.Set("state", r.URL.Query().Get("state"))
		callbackURL.RawQuery = query.Encode()

		http.Redirect(w, r, callbackURL.String(), http.StatusMovedPermanently)
		return
	}
}

func handleToken(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"access_token": "foo",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleUserInfo(w http.ResponseWriter, r *http.Request) {
	// Example from https://cloud.google.com/docs/authentication/token-types#access-contents
	response := map[string]string{
		"azp":            "32553540559.apps.googleusercontent.com",
		"aud":            "32553540559.apps.googleusercontent.com",
		"sub":            "111260650121245072906",
		"scope":          "openid https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/accounts.reauth",
		"exp":            "1650056632",
		"expires_in":     "3488",
		"email":          "bob@op2.com",
		"email_verified": "true",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func main() {
	http.HandleFunc("/oauth2/auth", handleAuth)
	http.HandleFunc("/oauth2/token", handleToken)
	http.HandleFunc("/tokeninfo", handleUserInfo)

	servicePort := 8081

	log.Printf("Server started at :%d\n", servicePort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", servicePort), nil))
}
