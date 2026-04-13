package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// BusinessQuestionOperationsTotal счетчик операций с базой вопросов
	BusinessQuestionOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_business_question_operations_total",
			Help: "Total number of question operations",
		},
		[]string{"service", "operation", "status"},
	)
)

func init() {
	prometheus.MustRegister(BusinessQuestionOperationsTotal)
}
