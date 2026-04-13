package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// BusinessResultOperationsTotal счетчик операций с результатами
	BusinessResultOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_business_result_operations_total",
			Help: "Total number of result operations",
		},
		[]string{"service", "operation", "status"},
	)
)

func init() {
	prometheus.MustRegister(BusinessResultOperationsTotal)
}
