package reporting

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx"
	"github.com/jackc/pgx/v4/pgxpool"

	"github.com/TheLab-ms/access-controller-controller/client"
	"github.com/TheLab-ms/access-controller-controller/conf"
	"github.com/TheLab-ms/access-controller-controller/keycloak"
)

const migration = `
CREATE TABLE IF NOT EXISTS swipes (
	id integer primary key,
	cardID integer not null,
	doorID text not null,
	time timestamp not null,
	name text not null
);

ALTER TABLE swipes ADD COLUMN IF NOT EXISTS seenAt timestamp;

CREATE INDEX IF NOT EXISTS idx_swipes_cardID ON swipes (cardID);
CREATE INDEX IF NOT EXISTS idx_swipes_time ON swipes (time);
`

type Controller struct {
	db                  *pgxpool.Pool
	client              *client.Client
	keycloak            *keycloak.Keycloak
	swipeScrapeInterval time.Duration
}

func NewController(env *conf.Env, ac *client.Client, kc *keycloak.Keycloak) (*Controller, error) {
	db, err := pgxpool.Connect(context.Background(), fmt.Sprintf("user=%s password=%s host=%s port=5432 dbname=postgres", env.PostgresUser, env.PostgresPassword, env.PostgresHost))
	if err != nil {
		return nil, fmt.Errorf("constructing db client: %w", err)
	}

	_, err = db.Exec(context.Background(), migration)
	if err != nil {
		return nil, fmt.Errorf("db migration: %w", err)
	}

	return &Controller{
		db:                  db,
		client:              ac,
		keycloak:            kc,
		swipeScrapeInterval: env.SwipeScrapeInterval,
	}, nil
}

func (c *Controller) Run(ctx context.Context) {
	runLoop(c.swipeScrapeInterval, func() bool {
		err := c.scrape(ctx)
		if err != nil {
			log.Printf("error scraping swipe events: %s", err)
		}
		return err == nil
	})
}

func (c *Controller) scrape(ctx context.Context) error {
	start := time.Now()
	log.Printf("starting to scrape swipe events")
	defer log.Printf("finished scraping swipe events in %s", time.Since(start))

	var queryStart int64
	err := c.db.QueryRow(context.Background(), "SELECT id FROM swipes ORDER BY id DESC LIMIT 1").Scan(&queryStart)
	if errors.Is(err, pgx.ErrNoRows) {
		queryStart = -1
		err = nil
	}
	if err != nil {
		return fmt.Errorf("finding cursor position: %s", err)
	}
	log.Printf("last known swipe event ID: %d", queryStart)

	usersByUUID := map[string]*keycloak.AccessUser{}
	if c.keycloak != nil {
		allUsers, err := c.keycloak.ListUsers(ctx)
		if err != nil {
			return fmt.Errorf("listing users from Keycloak: %w", err)
		}
		for _, user := range allUsers {
			uuid := strings.ReplaceAll(user.UUID, "-", "") // remove dashes since we don't store them in access controller
			usersByUUID[uuid] = user
		}
	}

	fn := func(swipe *client.CardSwipe) error {
		var name string
		if user := usersByUUID[swipe.Name]; user != nil {
			name = user.Name
		} else {
			name = swipe.Name // fall back to UUID
		}

		_, err := c.db.Exec(ctx, "INSERT INTO swipes (id, cardID, doorID, time, name, seenAt) VALUES ($1, $2, $3, $4, $5, NOW()) ON CONFLICT DO NOTHING", swipe.ID, swipe.CardID, swipe.DoorID, swipe.Time, name)
		if err != nil {
			return fmt.Errorf("inserting swipe %d into database: %s", swipe.ID, err)
		}

		log.Printf("inserted swipe event %d into database - card=%d door=%s time=%s", swipe.ID, swipe.CardID, swipe.DoorID, swipe.Time)
		return nil
	}

	return c.client.ListSwipes(ctx, int(queryStart), fn)
}

func runLoop(interval time.Duration, fn func() bool) {
	var lastRetry time.Duration
	for {
		if fn() {
			lastRetry = 0
			time.Sleep(interval)
			continue
		}

		if lastRetry == 0 {
			lastRetry = time.Millisecond * 250
		}
		lastRetry += lastRetry / 2
		if lastRetry > time.Hour {
			lastRetry = time.Hour
		}
		time.Sleep(lastRetry)
	}
}
