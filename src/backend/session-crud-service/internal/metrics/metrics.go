package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// BusinessSessionOperationsTotal счетчик операций с сессиями
	BusinessSessionOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_business_session_operations_total",
			Help: "Total number of session operations",
		},
		[]string{"service", "operation", "status"},
	)
)

func init() {
	prometheus.MustRegister(BusinessSessionOperationsTotal)
}
