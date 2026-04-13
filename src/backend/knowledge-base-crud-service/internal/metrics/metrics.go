package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// BusinessKnowledgeOperationsTotal счетчик операций с базой знаний
	BusinessKnowledgeOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_business_knowledge_operations_total",
			Help: "Total number of knowledge operations",
		},
		[]string{"service", "operation", "status"},
	)
)

func init() {
	prometheus.MustRegister(BusinessKnowledgeOperationsTotal)
}
