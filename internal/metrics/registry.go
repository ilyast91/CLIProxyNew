package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/usage"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry объединяет изолированные Prometheus collectors бизнес-слоя.
type Registry struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	latency  *prometheus.HistogramVec
}

// NewRegistry создаёт registry для HTTP, upstream, usage и PostgreSQL pool метрик.
func NewRegistry(pool *pgxpool.Pool, hook *usage.Hook, queue *usage.BufferedPlugin) *Registry {
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "cliproxy",
		Name:      "http_requests_total",
		Help:      "Количество HTTP-запросов по маршруту и status code.",
	}, []string{"method", "path", "status"})
	latency := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "cliproxy",
		Name:      "http_request_duration_seconds",
		Help:      "Длительность HTTP-запросов по маршруту.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "path"})
	registry := prometheus.NewRegistry()
	registry.MustRegister(requests, latency, newUpstreamCollector(hook))
	registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: "cliproxy",
		Name:      "usage_queue_depth",
		Help:      "Текущее число событий в очереди usage analytics.",
	}, func() float64 {
		if queue == nil {
			return 0
		}
		return float64(queue.QueueDepth())
	}))
	if pool != nil {
		registry.MustRegister(newDBPoolCollector(pool))
	}
	return &Registry{registry: registry, requests: requests, latency: latency}
}

// Handler возвращает HTTP handler Prometheus exposition format.
func (r *Registry) Handler() http.Handler {
	if r == nil || r.registry == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}

// Middleware учитывает HTTP-метрики, используя только method, route template и status code.
func (r *Registry) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if r == nil || r.requests == nil || r.latency == nil {
			c.Next()
			return
		}
		startedAt := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		method := c.Request.Method
		status := strconv.Itoa(c.Writer.Status())
		r.requests.WithLabelValues(method, path, status).Inc()
		r.latency.WithLabelValues(method, path).Observe(time.Since(startedAt).Seconds())
	}
}

type upstreamCollector struct {
	hook      *usage.Hook
	results   *prometheus.Desc
	lifecycle *prometheus.Desc
}

func newUpstreamCollector(hook *usage.Hook) *upstreamCollector {
	return &upstreamCollector{
		hook:      hook,
		results:   prometheus.NewDesc("cliproxy_upstream_results_total", "Количество завершённых upstream-вызовов.", []string{"outcome"}, nil),
		lifecycle: prometheus.NewDesc("cliproxy_upstream_auth_lifecycle_total", "Количество lifecycle-событий upstream credentials.", []string{"event"}, nil),
	}
}

func (c *upstreamCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.results
	ch <- c.lifecycle
}

func (c *upstreamCollector) Collect(ch chan<- prometheus.Metric) {
	var snapshot usage.HookSnapshot
	if c.hook != nil {
		snapshot = c.hook.Snapshot()
	}
	ch <- prometheus.MustNewConstMetric(c.results, prometheus.CounterValue, float64(snapshot.Succeeded), "success")
	ch <- prometheus.MustNewConstMetric(c.results, prometheus.CounterValue, float64(snapshot.Failed), "failure")
	ch <- prometheus.MustNewConstMetric(c.lifecycle, prometheus.CounterValue, float64(snapshot.AuthRegistered), "registered")
	ch <- prometheus.MustNewConstMetric(c.lifecycle, prometheus.CounterValue, float64(snapshot.AuthUpdated), "updated")
}

type dbPoolCollector struct {
	pool        *pgxpool.Pool
	connections *prometheus.Desc
	acquires    *prometheus.Desc
}

func newDBPoolCollector(pool *pgxpool.Pool) *dbPoolCollector {
	return &dbPoolCollector{
		pool:        pool,
		connections: prometheus.NewDesc("cliproxy_db_pool_connections", "Состояние соединений PostgreSQL pool.", []string{"state"}, nil),
		acquires:    prometheus.NewDesc("cliproxy_db_pool_acquires_total", "Попытки получить соединение из PostgreSQL pool.", []string{"outcome"}, nil),
	}
}

func (c *dbPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.connections
	ch <- c.acquires
}

func (c *dbPoolCollector) Collect(ch chan<- prometheus.Metric) {
	if c.pool == nil {
		return
	}
	stats := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(stats.AcquiredConns()), "acquired")
	ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(stats.IdleConns()), "idle")
	ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(stats.TotalConns()), "total")
	ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(stats.MaxConns()), "max")
	ch <- prometheus.MustNewConstMetric(c.acquires, prometheus.CounterValue, float64(stats.AcquireCount()), "acquired")
	ch <- prometheus.MustNewConstMetric(c.acquires, prometheus.CounterValue, float64(stats.CanceledAcquireCount()), "cancelled")
}
