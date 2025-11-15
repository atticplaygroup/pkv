package api

import (
	connectcors "connectrpc.com/cors"
	"github.com/rs/cors"
)

func GetCorsConfig() *cors.Cors {
	// Allow all origin access to essentially disable CORS check.
	// The security may need further discussion, but the reason to do so is
	// if all requests are made by the user agent, it already takes expenditure
	// into account. Then quota consumption from any origin is intended.
	return cors.New(cors.Options{
		AllowOriginFunc:  func(origin string) bool { return true },
		AllowedMethods:   connectcors.AllowedMethods(),
		AllowedHeaders:   append(connectcors.AllowedHeaders(), "Authorization"),
		ExposedHeaders:   connectcors.ExposedHeaders(),
		AllowCredentials: true,
		// Debug:            true,
	})
}
