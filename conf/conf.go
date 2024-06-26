package conf

import (
	"time"
)

type Env struct {
	AccessControlHost    string        `required:"true" split_words:"true"`
	AccessControlTimeout time.Duration `default:"5s" split_words:"true"`

	PostgresHost     string `split_words:"true"`
	PostgresUser     string `default:"postgres" split_words:"true"`
	PostgresPassword string `split_words:"true"`

	KeycloakURL       string `split_words:"true"`
	KeycloakRealm     string `default:"master" split_words:"true"`
	AuthorizedGroupID string `split_words:"true"`

	ResyncInterval time.Duration `default:"1h" split_words:"true"`
	CallbackURL    string        `split_words:"true"`
	WebhookAddr    string        `split_words:"true"`

	ProbeAddr           string        `default:":8888" split_words:"true"`
	SwipeScrapeInterval time.Duration `default:"2h" split_words:"true"`
}
