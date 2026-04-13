package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// KafkaMessagesProducedTotal счетчик отправленных сообщений в Kafka
	KafkaMessagesProducedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_kafka_messages_produced_total",
			Help: "Total number of Kafka messages produced",
		},
		[]string{"service", "topic", "event_type", "status"},
	)

	// KafkaMessagesConsumedTotal счетчик полученных сообщений из Kafka
	KafkaMessagesConsumedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_kafka_messages_consumed_total",
			Help: "Total number of Kafka messages consumed",
		},
		[]string{"service", "topic", "event_type", "status"},
	)

	// KafkaMessageProcessingDuration длительность обработки сообщений
	KafkaMessageProcessingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tensortalks_kafka_message_processing_duration_seconds",
			Help:    "Kafka message processing duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"service", "event_type"},
	)
)

func init() {
	prometheus.MustRegister(KafkaMessagesProducedTotal)
	prometheus.MustRegister(KafkaMessagesConsumedTotal)
	prometheus.MustRegister(KafkaMessageProcessingDuration)
}
