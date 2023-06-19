package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx"
	"github.com/kelseyhightower/envconfig"

	"github.com/TheLab-ms/access-controller-controller/client"
)

const migration = `
CREATE TABLE IF NOT EXISTS swipes (
	id integer PRIMARY KEY,
	cardID integer NOT NULL,
	time integer NOT NULL,
	name TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_swipes_cardID ON swipes (cardID);
CREATE INDEX IF NOT EXISTS idx_swipes_time ON swipes (time);
`

type config struct {
	AccessControlHost    string        `required:"true" split_words:"true"`
	AccessControlTimeout time.Duration `default:"5s" split_words:"true"`

	PostgresHost     string `required:"true" split_words:"true"`
	PostgresUser     string `default:"postgres" split_words:"true"`
	PostgresPassword string `required:"true" split_words:"true"`
}

func main() {
	conf := &config{}
	if err := envconfig.Process("", conf); err != nil {
		panic(err)
	}

	cli := &client.Client{
		Addr:    conf.AccessControlHost,
		Timeout: time.Second * 5,
	}

	db, err := pgx.Connect(pgx.ConnConfig{
		Host:     conf.PostgresHost,
		User:     conf.PostgresUser,
		Password: conf.PostgresPassword,
	})
	if err != nil {
		panic(err)
	}
	defer db.Close()

	_, err = db.Exec(migration)
	if err != nil {
		panic(err)
	}

	runLoop(time.Hour*8, func() bool {
		err := scrapeSwipes(context.Background(), cli, db)
		if err != nil {
			log.Printf("error scraping swipe events: %s", err)
		}
		return err == nil
	})
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

func scrapeSwipes(ctx context.Context, cli *client.Client, db *pgx.Conn) error {
	start := time.Now()
	log.Printf("starting to scrape swipe events")
	defer log.Printf("finished scraping swipe events in %s", time.Since(start))

	var queryStart int64
	err := db.QueryRow("SELECT id FROM swipes ORDER BY id DESC LIMIT 1").Scan(&queryStart)
	if errors.Is(err, pgx.ErrNoRows) {
		queryStart = -1
		err = nil
	}
	if err != nil {
		return fmt.Errorf("finding cursor position: %s", err)
	}
	log.Printf("last known swipe event ID: %d", queryStart)

	fn := func(swipe *client.CardSwipe) error {
		if swipe.Status == "Reboot" || !strings.Contains(swipe.Status, "Allow IN") {
			return nil // skip reboots and failed swipes
		}

		_, err := db.Exec("INSERT INTO swipes (id, cardID, time, name) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING", swipe.ID, swipe.CardID, swipe.Time, swipe.Name)
		if err != nil {
			return fmt.Errorf("inserting swipe %d into database: %s", swipe.ID, err)
		}

		log.Printf("inserted swipe event %d into database", swipe.ID)
		return nil
	}

	return cli.ListSwipes(ctx, int(queryStart), fn)
}
