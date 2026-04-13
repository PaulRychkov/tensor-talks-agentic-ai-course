package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// BusinessChatOperationsTotal счетчик операций с чатами
	BusinessChatOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_business_chat_operations_total",
			Help: "Total number of chat operations",
		},
		[]string{"service", "operation", "status"},
	)
)

func init() {
	prometheus.MustRegister(BusinessChatOperationsTotal)
}
