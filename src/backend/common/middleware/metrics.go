package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HTTPRequestsTotal счетчик HTTP-запросов
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tensortalks_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"service", "method", "endpoint", "status_code"},
	)

	// HTTPRequestDuration гистограмма длительности HTTP-запросов
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tensortalks_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"service", "method", "endpoint"},
	)

	// HTTPRequestSizeBytes гистограмма размера тела запроса
	HTTPRequestSizeBytes = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tensortalks_http_request_size_bytes",
			Help:    "HTTP request size in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 10, 7),
		},
		[]string{"service", "method", "endpoint"},
	)

	// HTTPResponseSizeBytes гистограмма размера тела ответа
	HTTPResponseSizeBytes = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tensortalks_http_response_size_bytes",
			Help:    "HTTP response size in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 10, 7),
		},
		[]string{"service", "method", "endpoint"},
	)
)

func init() {
	prometheus.MustRegister(HTTPRequestsTotal)
	prometheus.MustRegister(HTTPRequestDuration)
	prometheus.MustRegister(HTTPRequestSizeBytes)
	prometheus.MustRegister(HTTPResponseSizeBytes)
}

// PrometheusMiddleware создаёт middleware для сбора HTTP-метрик
func PrometheusMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		// Размер запроса
		requestSize := float64(c.Request.ContentLength)
		if requestSize < 0 {
			requestSize = 0
		}

		// Обработка запроса
		c.Next()

		// Время обработки
		duration := time.Since(start).Seconds()
		statusCode := strconv.Itoa(c.Writer.Status())

		// Размер ответа
		responseSize := float64(c.Writer.Size())

		// Записываем метрики
		HTTPRequestsTotal.WithLabelValues(
			serviceName,
			c.Request.Method,
			path,
			statusCode,
		).Inc()

		HTTPRequestDuration.WithLabelValues(
			serviceName,
			c.Request.Method,
			path,
		).Observe(duration)

		if requestSize > 0 {
			HTTPRequestSizeBytes.WithLabelValues(
				serviceName,
				c.Request.Method,
				path,
			).Observe(requestSize)
		}

		if responseSize > 0 {
			HTTPResponseSizeBytes.WithLabelValues(
				serviceName,
				c.Request.Method,
				path,
			).Observe(responseSize)
		}
	}
}

// MetricsHandler возвращает handler для /metrics endpoint
func MetricsHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
