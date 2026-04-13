package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// DBQueriesTotal счетчик запросов к БД
	DBQueriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_db_queries_total",
			Help: "Total number of database queries",
		},
		[]string{"service", "operation", "status"},
	)

	// DBQueryDuration длительность запросов к БД
	DBQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tensortalks_db_query_duration_seconds",
			Help:    "Database query duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"service", "operation"},
	)

	// DBErrorsTotal счетчик ошибок БД
	DBErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_db_errors_total",
			Help: "Total number of database errors",
		},
		[]string{"service", "error_type"},
	)

	// BusinessUserOperationsTotal счетчик операций с пользователями
	BusinessUserOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_business_user_operations_total",
			Help: "Total number of user operations",
		},
		[]string{"service", "operation", "status"},
	)
)

func init() {
	prometheus.MustRegister(DBQueriesTotal)
	prometheus.MustRegister(DBQueryDuration)
	prometheus.MustRegister(DBErrorsTotal)
	prometheus.MustRegister(BusinessUserOperationsTotal)
}
