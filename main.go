package main

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cenkalti/backoff"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/olivere/elastic.v2"

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

func remove(c *Config) error {
	client, err := elastic.NewClient(
		elastic.SetURL(c.URL()),
		elastic.SetMaxRetries(10),
	)
	if err != nil {
		return err
	}

	in, err := client.IndexNames()
	if err != nil {
		return err
	}
	var victims []string
	for _, iname := range in {
		if strings.HasPrefix(iname, c.Prefix) {
			victims = append(victims, iname)
		}
	}
	sort.Strings(victims)
	for i := len(victims) - (c.Retention + 1); i >= 0; i-- {
		iname := victims[i]
		_, err := client.DeleteIndex(iname).Do()
		if err != nil {
			log.Printf("Failed to delete index %s: %s", iname, err)
			continue
		}
		log.Printf("Deleted index %s", iname)
		deleted.Inc()
	}
	return nil
}

func main() {
	log.Infof("Starting %s version %s (build at %s from %s)", Name, Version, BuildTime, Commit)
	cfg, err := New()
	if err != nil {
		log.Fatalf("Error: %s", err)
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

	go listen()

	interval := time.Hour * time.Duration(cfg.Interval)
	for {
		t0 := time.Now()
		opFunc := func() error {
			return remove(cfg)
		}
		logFunc := func(err error, wait time.Duration) {
			log.Printf("Failed to connect to ES at %s: %s. Retry in %s", cfg.URL(), err, wait)
		}
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = time.Second
		bo.MaxInterval = 60 * time.Second
		bo.MaxElapsedTime = 15 * time.Minute
		err := backoff.RetryNotify(opFunc, bo, logFunc)
		if err != nil {
			runs.WithLabelValues("failed").Inc()
			log.Printf("Failed to delete indices: %s", err)
			continue
		}
		runs.WithLabelValues("ok").Inc()
		d0 := float64(time.Since(t0)) / float64(time.Microsecond)
		duration.WithLabelValues("delete").Observe(d0)
		if interval < time.Second {
			break
		}
		log.Printf("Waiting %s until next run", interval.String())
		time.Sleep(interval)
	}
	os.Exit(0)
}

func listen() {
	listen := os.Getenv("LISTEN")
	if listen == "" {
		listen = ":8080"
	}
	s := &http.Server{
		Addr:    listen,
		Handler: requestHandler(),
	}
	log.Printf("Listening on %s", listen)
	log.Errorf("Failed to listen on %s: %s", listen, s.ListenAndServe())
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
