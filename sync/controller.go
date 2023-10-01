package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/TheLab-ms/access-controller-controller/client"
	"github.com/TheLab-ms/access-controller-controller/conf"
	"github.com/TheLab-ms/access-controller-controller/keycloak"
)

type accessController interface {
	AddCard(ctx context.Context, num int, name string) error
	RemoveCard(ctx context.Context, id int) error
	ListCards(ctx context.Context) ([]*client.Card, error)
}

type userStorage interface {
	ListUsers(ctx context.Context) ([]*keycloak.AccessUser, error)
	CreateWebhook(ctx context.Context, webhook *keycloak.Webhook) error
	ListWebhooks(ctx context.Context) ([]*keycloak.Webhook, error)
}

type Controller struct {
	controller accessController
	storage    userStorage
	conf       *conf.Env
	trigger    chan struct{}
}

func NewController(c *conf.Env, cli *client.Client, kc *keycloak.Keycloak) *Controller {
	ctrl := &Controller{
		controller: cli,
		storage:    kc,
		conf:       c,
		trigger:    make(chan struct{}, 1),
	}
	ctrl.trigger <- struct{}{} // sync when starting up
	return ctrl
}

func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Provide a super secret debug endpoint for listing the current cards
	if r.URL.Path == "/cards" {
		log.Printf("received list cards request")
		cards, err := c.controller.ListCards(r.Context())
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(&cards)
		return
	}

	if !strings.HasPrefix(r.URL.Path, "/webhook") {
		return
	}
	log.Printf("received webhook")
	select {
	case c.trigger <- struct{}{}:
	default:
	}
}

func (c *Controller) Run(ctx context.Context) {
	// Sync periodically
	go func() {
		for range time.NewTicker(c.conf.ResyncInterval).C {
			select {
			case c.trigger <- struct{}{}:
			default:
			}
		}
	}()

	var lastRetry time.Duration
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.trigger:
		}

	start:
		changed, err := c.sync(ctx)
		if err != nil {
			log.Printf("sync error: %s", err)
		} else {
			lastRetry = 0

			if !changed {
				time.Sleep(time.Second * 5) // cooldown
				continue
			}
		}

		if lastRetry == 0 {
			lastRetry = time.Millisecond * 250
		}
		lastRetry += lastRetry / 2
		if lastRetry > time.Hour {
			lastRetry = time.Hour
		}
		time.Sleep(lastRetry)
		goto start
	}
}

func (c *Controller) sync(ctx context.Context) (bool, error) {
	goalUsers, err := c.storage.ListUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("listing users from storage: %w", err)
	}

	cards, err := c.controller.ListCards(ctx)
	if err != nil {
		return false, fmt.Errorf("listing cards from access controller: %w", err)
	}

	usersByFobID := map[int]*keycloak.AccessUser{}
	for _, user := range goalUsers {
		usersByFobID[user.KeyfobNumber] = user
	}

	// Clean up unused or incorrectly attributed cards
	cardsByFobNumber := map[int]*client.Card{}
	for _, card := range cards {
		cardsByFobNumber[card.Number] = card

		// Assume that names not managed by this tool are "First Last" and thus will contain a space
		isManaged := !strings.Contains(card.Name, " ")

		user := usersByFobID[card.Number]
		if (user == nil && !isManaged) || (user != nil && trimDashes(user.UUID) == card.Name) {
			continue
		}

		err := c.controller.RemoveCard(ctx, card.ID)
		if err != nil {
			return false, fmt.Errorf("removing card %d from controller: %s", card.ID, err)
		}

		log.Printf("removed card %d from the controller", card.ID)
		return true, nil
	}

	// Create missing cards
	for _, user := range goalUsers {
		_, ok := cardsByFobNumber[user.KeyfobNumber]
		if ok {
			continue // already exists
		}

		err := c.controller.AddCard(ctx, user.KeyfobNumber, trimDashes(user.UUID))
		if err != nil {
			return false, fmt.Errorf("adding card for user %s: %s", user.UUID, err)
		}

		log.Printf("associated card %d with user %s", user.KeyfobNumber, user.UUID)
		return true, nil
	}

	return false, nil
}

func (c *Controller) EnsureWebhook(ctx context.Context) error {
	hooks, err := c.storage.ListWebhooks(ctx)
	if err != nil {
		return fmt.Errorf("listing: %w", err)
	}

	url := fmt.Sprintf("%s/webhook", c.conf.CallbackURL)
	for _, hook := range hooks {
		if hook.URL == url {
			return nil // already exists
		}
	}

	return c.storage.CreateWebhook(ctx, &keycloak.Webhook{
		Enabled:    true,
		URL:        url,
		EventTypes: []string{"admin.*"},
	})
}

// trimDashes removes dashes from a uuid, which is necessary because the controller doesn't allow dashes in card names.
func trimDashes(uuid string) string {
	return strings.ReplaceAll(uuid, "-", "")
}
