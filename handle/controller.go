package handle

import (
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

// InitializeRoutes The init function for all routes
func InitializeRoutes(Router *mux.Router) {
	Router.Use(handlers.RecoveryHandler())
	Router.Handle("/", health()).Methods("GET")
	Router.Handle("/health_check", health()).Methods("GET")

	pushSubRouter := Router.PathPrefix("/get").Subrouter()
	pushSubRouter.Handle("/{carrier_moniker}/{tracking_number}", parser()).Methods("GET")
}
