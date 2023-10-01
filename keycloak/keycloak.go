package keycloak

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/Nerzal/gocloak/v13"

	"github.com/TheLab-ms/access-controller-controller/conf"
)

type Keycloak struct {
	client            *gocloak.GoCloak
	user, pass, realm string
	baseURL, groupID  string

	// use ensureToken to access these
	tokenLock      sync.Mutex
	token          *gocloak.JWT
	tokenFetchTime time.Time
}

func New(c *conf.Env) *Keycloak {
	return &Keycloak{client: gocloak.NewClient(c.KeycloakURL), user: c.KeycloakUser, pass: c.KeycloakPassword, realm: c.KeycloakRealm, baseURL: c.KeycloakURL, groupID: c.AuthorizedGroupID}
}

func (k *Keycloak) ListUsers(ctx context.Context) ([]*AccessUser, error) {
	token, err := k.ensureToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting token: %w", err)
	}

	var (
		max   = 50
		first = 0
		all   = []*AccessUser{}
	)
	for {
		params, err := gocloak.GetQueryParams(gocloak.GetUsersParams{
			Max:   &max,
			First: &first,
		})
		if err != nil {
			return nil, err
		}

		// Unfortunately the keycloak client doesn't support the group membership endpoint.
		// We reuse the client's transport here while specifying our own URL.
		var users []*gocloak.User
		_, err = k.client.GetRequestWithBearerAuth(ctx, token.AccessToken).
			SetResult(&users).
			SetQueryParams(params).
			Get(fmt.Sprintf("%s/admin/realms/%s/groups/%s/members", k.baseURL, k.realm, k.groupID))
		if err != nil {
			return nil, err
		}
		if len(users) == 0 {
			break
		}
		first += len(users)

		for _, user := range users {
			u := newAccessUser(user)
			if u == nil {
				continue // invalid user (should be impossible)
			}
			all = append(all, u)
		}
	}

	return all, nil
}

func (k *Keycloak) ListWebhooks(ctx context.Context) ([]*Webhook, error) {
	token, err := k.ensureToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting token: %w", err)
	}

	webhooks := []*Webhook{}
	_, err = k.client.GetRequestWithBearerAuth(ctx, token.AccessToken).
		SetResult(&webhooks).
		Get(fmt.Sprintf("%s/realms/%s/webhooks", k.baseURL, k.realm))
	if err != nil {
		return nil, err
	}

	return webhooks, nil
}

func (k *Keycloak) CreateWebhook(ctx context.Context, webhook *Webhook) error {
	token, err := k.ensureToken(ctx)
	if err != nil {
		return fmt.Errorf("getting token: %w", err)
	}

	_, err = k.client.GetRequestWithBearerAuth(ctx, token.AccessToken).
		SetBody(webhook).
		Post(fmt.Sprintf("%s/realms/%s/webhooks", k.baseURL, k.realm))
	if err != nil {
		return err
	}

	return nil
}

// For whatever reason the Keycloak client doesn't support token rotation
func (k *Keycloak) ensureToken(ctx context.Context) (*gocloak.JWT, error) {
	k.tokenLock.Lock()
	defer k.tokenLock.Unlock()

	if k.token != nil && time.Since(k.tokenFetchTime) < (time.Duration(k.token.ExpiresIn)*time.Second)/2 {
		return k.token, nil
	}

	token, err := k.client.LoginAdmin(ctx, k.user, k.pass, k.realm)
	if err != nil {
		return nil, err
	}
	k.token = token
	k.tokenFetchTime = time.Now()

	log.Printf("fetched new auth token from keycloak - will expire in %d seconds", k.token.ExpiresIn)
	return k.token, nil
}

type AccessUser struct {
	UUID, Name   string
	KeyfobNumber int
}

func newAccessUser(kcuser *gocloak.User) *AccessUser {
	if kcuser.ID == nil || kcuser.Attributes == nil {
		return nil
	}

	attr := *kcuser.Attributes
	fobID, _ := strconv.Atoi(firstElOrZeroVal(attr["keyfobID"]))
	if fobID == 0 {
		return nil
	}

	return &AccessUser{
		UUID:         *kcuser.ID,
		Name:         fmt.Sprintf("%s %s", gocloak.PString(kcuser.FirstName), gocloak.PString(kcuser.LastName)),
		KeyfobNumber: fobID,
	}
}

type Webhook struct {
	ID         string   `json:"id"`
	Enabled    bool     `json:"enabled"`
	URL        string   `json:"url"`
	EventTypes []string `json:"eventTypes"`
}

func firstElOrZeroVal[T any](slice []T) (val T) {
	if len(slice) == 0 {
		return val
	}
	return slice[0]
}
