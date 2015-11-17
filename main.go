package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cenkalti/backoff"
	"gopkg.in/olivere/elastic.v2"

	"github.com/alexflint/go-arg"
	"github.com/danryan/env"
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
		} else {
			log.Printf("Deleted index %s", iname)
		}
	}
	return nil
}

func main() {
	cfg, err := New()
	if err != nil {
		panic(err)
	}
	interval := time.Hour * time.Duration(cfg.Interval)
	for {
		opFunc := func() error {
			return remove(cfg)
		}
		logFunc := func(err error, wait time.Duration) {
			log.Printf("Failed to connect to ES: %s. Retry in %s", err, wait)
		}
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = time.Second
		bo.MaxInterval = 60 * time.Second
		bo.MaxElapsedTime = 15 * time.Minute
		err := backoff.RetryNotify(opFunc, bo, logFunc)
		if err != nil {
			log.Printf("Failed to delete indices: %s", err)
			continue
		}
		if interval < time.Second {
			break
		}
		log.Printf("Waiting %s until next run", interval.String())
		time.Sleep(interval)
	}
	os.Exit(0)
}
