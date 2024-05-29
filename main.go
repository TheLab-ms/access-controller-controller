package main

import (
	"context"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/kelseyhightower/envconfig"

	"github.com/TheLab-ms/access-controller-controller/client"
	"github.com/TheLab-ms/access-controller-controller/conf"
	"github.com/TheLab-ms/access-controller-controller/keycloak"
	"github.com/TheLab-ms/access-controller-controller/reporting"
	"github.com/TheLab-ms/access-controller-controller/sync"
)

func main() {
	ctx := context.Background()
	conf := &conf.Env{}
	if err := envconfig.Process("", conf); err != nil {
		panic(err)
	}

	cli := &client.Client{
		Addr:    conf.AccessControlHost,
		Timeout: conf.AccessControlTimeout,
	}
	probe := &livenessProbe{}

	// Sync badge access from keycloak if configured
	var kc *keycloak.Keycloak
	if conf.KeycloakURL == "" {
		log.Printf("disabling keyvault sync because keycloak URL is not set")
	} else {
		kc = keycloak.New(conf)
		c := sync.NewController(conf, cli, kc)

		if conf.CallbackURL != "" {
			err := c.EnsureWebhook(ctx)
			if err != nil {
				log.Fatalf("error while ensuring webhook resource exists: %s", err)
			}
		}

		if conf.WebhookAddr != "" {
			go func() {
				if err := http.ListenAndServe(conf.WebhookAddr, c); err != nil {
					log.Fatalf("error while starting webhook listener: %s", err)
				}
			}()
		}

		probe.Add(&c.LastSync)
		go c.Run(ctx)
	}

	// Scrape badge swipes to the reporting database if configured
	if conf.SwipeScrapeInterval == 0 {
		log.Printf("disabling reporting controller because swipe scrape interval is zero")
	} else {
		ctrl, err := reporting.NewController(conf, cli, kc)
		if err != nil {
			log.Fatalf("error while configuring reporting controller: %s", err)
		}
		probe.Add(&ctrl.LastSync)
		go ctrl.Run(ctx)
	}

	if conf.ProbeAddr != "" {
		go func() {
			http.ListenAndServe(conf.ProbeAddr, probe)
		}()
	}

	<-ctx.Done() // sleep forever while things run in other goroutines
}

// This is a very crude probe to kick the process if the loops get stuck for some reason.
type livenessProbe struct {
	checks []*atomic.Pointer[time.Time]
}

func (l *livenessProbe) Add(ptr *atomic.Pointer[time.Time]) { l.checks = append(l.checks, ptr) }

func (l *livenessProbe) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var mostRecent time.Time
	for _, check := range l.checks {
		ts := check.Load()
		if ts != nil && ts.After(mostRecent) {
			mostRecent = *ts
		}
	}

	if time.Since(mostRecent) > time.Minute*2 {
		log.Printf("failing liveness probe!")
		w.WriteHeader(500)
	}
	w.WriteHeader(200)
}
