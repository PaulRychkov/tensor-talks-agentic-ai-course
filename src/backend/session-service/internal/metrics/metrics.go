package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// BusinessSessionsCreatedTotal счетчик созданных сессий
	BusinessSessionsCreatedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_business_sessions_created_total",
			Help: "Total number of sessions created",
		},
		[]string{"service", "status"},
	)

	// BusinessSessionsOperationsTotal счетчик операций с сессиями
	BusinessSessionsOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_business_sessions_operations_total",
			Help: "Total number of session operations",
		},
		[]string{"service", "operation", "status"},
	)
)

func init() {
	prometheus.MustRegister(BusinessSessionsCreatedTotal)
	prometheus.MustRegister(BusinessSessionsOperationsTotal)
}
