package reporting

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx"

	"github.com/TheLab-ms/access-controller-controller/client"
	"github.com/TheLab-ms/access-controller-controller/conf"
)

const migration = `
CREATE TABLE IF NOT EXISTS swipes (
	id integer primary key,
	cardID integer not null,
	doorID text not null,
	time timestamp not null,
	name text not null
);

CREATE INDEX IF NOT EXISTS idx_swipes_cardID ON swipes (cardID);
CREATE INDEX IF NOT EXISTS idx_swipes_time ON swipes (time);
`

type Controller struct {
	db                  *pgx.Conn
	client              *client.Client
	swipeScrapeInterval time.Duration
}

func NewController(env *conf.Env, ac *client.Client) (*Controller, error) {
	db, err := pgx.Connect(pgx.ConnConfig{
		Host:     env.PostgresHost,
		User:     env.PostgresUser,
		Password: env.PostgresPassword,
	})
	if err != nil {
		return nil, fmt.Errorf("constructing db client: %w", err)
	}

	_, err = db.Exec(migration)
	if err != nil {
		return nil, fmt.Errorf("db migration: %w", err)
	}

	return &Controller{
		db:                  db,
		client:              ac,
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
	err := c.db.QueryRow("SELECT id FROM swipes ORDER BY id DESC LIMIT 1").Scan(&queryStart)
	if errors.Is(err, pgx.ErrNoRows) {
		queryStart = -1
		err = nil
	}
	if err != nil {
		return fmt.Errorf("finding cursor position: %s", err)
	}
	log.Printf("last known swipe event ID: %d", queryStart)

	fn := func(swipe *client.CardSwipe) error {
		_, err := c.db.Exec("INSERT INTO swipes (id, cardID, doorID, time, name) VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING", swipe.ID, swipe.CardID, swipe.DoorID, swipe.Time, swipe.Name)
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
