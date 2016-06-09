package main

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/olivere/elastic.v3"

	"github.com/alexflint/go-arg"
	"github.com/danryan/env"
)

var (
	deleted   prometheus.Counter
	Name      = "es-maint-index"
	Version   string
	BuildTime string
	Commit    string
)

type Config struct {
	Host      string `env:"key=HOST default=localhost:9200" arg:"--host"`
	Retention int    `env:"key=KEEP default=7" arg:"--keep"`
	Prefix    string `env:"key=PREFIX default=logstash-" arg:"--prefix"`
	Interval  int    `env:"key=INTERVAL default=24" arg:"--interval"`
}

func New() (*Config, error) {
	c := Config{}
	if err := env.Process(&c); err != nil {
		return nil, err
	}
	arg.MustParse(&c)
	return &c, nil
}

func (c *Config) URL() string {
	return fmt.Sprintf("http://%s", c.Host)
}

func remove(c *Config, l log.Logger) error {
	client, err := elastic.NewClient(
		elastic.SetURL(c.URL()),
		elastic.SetMaxRetries(10),
	)
	if err != nil {
		return fmt.Errorf("Failed to connect to ES: %s", err)
	}

	in, err := client.IndexNames()
	if err != nil {
		return fmt.Errorf("Failed to list indices: %s", err)
	}

	for _, prefix := range strings.Split(c.Prefix, ",") {
		var victims []string
		for _, iname := range in {
			if strings.HasPrefix(iname, prefix) {
				victims = append(victims, iname)
			}
		}
		sort.Strings(victims)
		l.Log("level", "debug", "msg", "Prefix Status", "prefix", prefix, "num_victims", len(victims), "retention", c.Retention, "victims", victims)
		for i := len(victims) - (c.Retention + 1); i >= 0; i-- {
			iname := victims[i]
			_, err := client.DeleteIndex(iname).Do()
			if err != nil {
				l.Log("level", "error", "msg", "Failed to delete index", "index", iname, "err", err)
				continue
			}
			l.Log("level", "info", "msg", "Deleted index", "index", iname)
			deleted.Inc()
		}
	}
	return nil
}

func main() {
	logger := log.NewLogfmtLogger(os.Stderr)
	if os.Getenv("ENVIRONMENT") == "prod" || os.Getenv("ENVIRONMENT") == "stage" {
		logger = log.NewJSONLogger(os.Stdout)
	}
	logger = log.NewContext(logger).With(
		"ts", log.DefaultTimestampUTC,
		"caller", log.DefaultCaller,
		"name", Name,
		"version", Version,
		"build_time", BuildTime,
		"commit", Commit,
	)
	logger.Log("level", "info", "msg", "Starting")

	cfg, err := New()
	if err != nil {
		logger.Log("level", "error", "msg", "Failed to parse config", "err", err)
		os.Exit(1)
	}

	runs := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "elasticsearch_index_maint_runs_total",
			Help: "Number of elasticsearch index maintenance runs",
		},
		[]string{"status"},
	)
	runs = prometheus.MustRegisterOrGet(runs).(*prometheus.CounterVec)
	deleted = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "elasticsearch_indices_deleted_total",
			Help: "Size of elasticsearch indices deleted",
		},
	)
	deleted = prometheus.MustRegisterOrGet(deleted).(prometheus.Counter)
	duration := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "elasticsearch_index_maint_duration",
			Help: "Duration of elasticsearch index maintenance runs",
		},
		[]string{"operation"},
	)
	duration = prometheus.MustRegisterOrGet(duration).(*prometheus.SummaryVec)

	go listen(logger)

	interval := time.Hour * time.Duration(cfg.Interval)
	for {
		t0 := time.Now()
		opFunc := func() error {
			return remove(cfg, logger)
		}
		logFunc := func(err error, wait time.Duration) {
			logger.Log("level", "warn", "msg", "Failed to connect to ES", "url", cfg.URL(), "err", err, "wait", wait)
		}
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = time.Second
		bo.MaxInterval = 60 * time.Second
		bo.MaxElapsedTime = 15 * time.Minute
		err := backoff.RetryNotify(opFunc, bo, logFunc)
		if err != nil {
			runs.WithLabelValues("failed").Inc()
			logger.Log("level", "error", "msg", "Failed to delete indices", "err", err)
			continue
		}
		runs.WithLabelValues("ok").Inc()
		d0 := float64(time.Since(t0)) / float64(time.Microsecond)
		duration.WithLabelValues("delete").Observe(d0)
		if interval < time.Second {
			break
		}
		logger.Log("level", "info", "msg", "Waiting until next run", "interval", interval.String())
		time.Sleep(interval)
	}
	os.Exit(0)
}

func listen(l log.Logger) {
	listen := os.Getenv("LISTEN")
	if listen == "" {
		listen = ":8080"
	}
	s := &http.Server{
		Addr:    listen,
		Handler: requestHandler(),
	}
	l.Log("level", "info", "msg", "Listening", "listen", listen)
	if err := s.ListenAndServe(); err != nil {
		l.Log("level", "error", "msg", "Failed to listen", "listen", listen, "err", err)
	}
}

func requestHandler() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/metrics", prometheus.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "OK", http.StatusOK)
	})
	mux.HandleFunc("/", http.NotFound)
	return mux
}
