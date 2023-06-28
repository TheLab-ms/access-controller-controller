package main

import (
	"context"
	"log"
	"net/http"

	"github.com/kelseyhightower/envconfig"

	"github.com/TheLab-ms/access-controller-controller/client"
	"github.com/TheLab-ms/access-controller-controller/conf"
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

	// Sync badge access from keycloak if configured
	if conf.KeycloakURL == "" {
		log.Printf("disabling keyvault sync because keycloak URL is not set")
	} else {
		c := sync.NewController(conf, cli)

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

		c.Run(ctx)
	}

	// Scrape badge swipes to the reporting database if configured
	if conf.SwipeScrapeInterval == 0 {
		log.Printf("disabling reporting controller because swipe scrape interval is zero")
	} else {
		ctrl, err := reporting.NewController(conf, cli)
		if err != nil {
			log.Fatalf("error while configuring reporting controller: %s", err)
		}
		go ctrl.Run(ctx)
	}

	<-ctx.Done() // sleep forever while things run in other goroutines
}
