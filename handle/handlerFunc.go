package handle

import (
	"net/http"
	"photoManager/constants"
	"photoManager/services"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

//Health Health Check controller
func health() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var healthCheck = "{\"status\": \"UP\"}"
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(healthCheck))
	})
}

//Health Health Check controller
func parser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		carrierMoniker := params[constants.CarrierMoniker]
		trackingNumber := params[constants.TrackingNumber]
		healthCheck := services.Parser(carrierMoniker, trackingNumber)
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(healthCheck))
	})
}

func amazonLogin() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		amazonCallbackUri := params["amazon_callback_uri"]
		amazonState := params["amazon_state"]
		version := params["version"]
		sellingPartnerId := params["selling_partner_id"]
		logrus.Infof("Data : %s, %s, %s, %s", amazonCallbackUri, amazonState, version, sellingPartnerId)
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		http.Redirect(w, r, "https://www.google.com", http.StatusSeeOther)
	})
}
