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

	// SessionUserRating гистограмма пользовательских оценок сессий (1-5)
	SessionUserRating = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "session_user_rating",
			Help:    "User satisfaction rating (1-5) collected after session completion",
			Buckets: []float64{1, 2, 3, 4, 5},
		},
		[]string{"session_kind"},
	)

	// SessionRatingsTotal счётчик полученных оценок
	SessionRatingsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "session_ratings_total",
			Help: "Total number of user ratings submitted",
		},
		[]string{"session_kind"},
	)

	// SessionCompletedTotal счётчик завершённых сессий (naturally vs terminated_early)
	SessionCompletedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "session_completed_total_crud",
			Help: "Sessions saved to results-crud: completed_naturally=true/false",
		},
		[]string{"session_kind", "completed_naturally"},
	)

	// ── Продуктовые метрики (обновляются фоновым воркером каждые 30 с) ──────

	// ProductTotalSessions — общее количество завершённых сессий в БД
	ProductTotalSessions = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tensortalks_product_total_sessions",
		Help: "Total sessions stored in results-crud",
	})

	// ProductCompletionRate — доля сессий, завершённых естественным образом (0–1)
	ProductCompletionRate = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tensortalks_product_completion_rate",
		Help: "Fraction of sessions completed naturally (not terminated early)",
	})

	// ProductAvgScore — средний балл по всем сессиям (0–100)
	ProductAvgScore = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tensortalks_product_avg_score",
		Help: "Average interview score across all sessions (0-100)",
	})

	// ProductAvgRating — средняя пользовательская оценка (1–5)
	ProductAvgRating = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tensortalks_product_avg_rating",
		Help: "Average user satisfaction rating (1-5)",
	})

	// ProductRatedSessions — количество сессий с пользовательской оценкой
	ProductRatedSessions = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "tensortalks_product_rated_sessions",
		Help: "Number of sessions that received a user rating",
	})

	// ProductSessionsByKind — количество сессий по режиму
	ProductSessionsByKind = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tensortalks_product_sessions_by_kind",
		Help: "Sessions count by session_kind (interview/training/study)",
	}, []string{"kind"})
)

func init() {
	prometheus.MustRegister(
		BusinessResultOperationsTotal,
		SessionUserRating,
		SessionRatingsTotal,
		SessionCompletedTotal,
		ProductTotalSessions,
		ProductCompletionRate,
		ProductAvgScore,
		ProductAvgRating,
		ProductRatedSessions,
		ProductSessionsByKind,
	)
}
