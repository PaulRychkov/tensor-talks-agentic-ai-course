package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// BusinessRegistrationsTotal счетчик регистраций
	BusinessRegistrationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_business_registrations_total",
			Help: "Total number of user registrations",
		},
		[]string{"service", "status"},
	)

	// BusinessLoginsTotal счетчик логинов
	BusinessLoginsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_business_logins_total",
			Help: "Total number of user logins",
		},
		[]string{"service", "status"},
	)

	// BusinessTokensIssuedTotal счетчик выданных токенов
	BusinessTokensIssuedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_business_tokens_issued_total",
			Help: "Total number of tokens issued",
		},
		[]string{"service", "token_type"},
	)

	// BusinessTokenValidationErrorsTotal счетчик ошибок валидации токенов
	BusinessTokenValidationErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_business_token_validation_errors_total",
			Help: "Total number of token validation errors",
		},
		[]string{"service", "error_type"},
	)
)

func init() {
	prometheus.MustRegister(BusinessRegistrationsTotal)
	prometheus.MustRegister(BusinessLoginsTotal)
	prometheus.MustRegister(BusinessTokensIssuedTotal)
	prometheus.MustRegister(BusinessTokenValidationErrorsTotal)
}
