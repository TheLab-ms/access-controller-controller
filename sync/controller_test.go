package sync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheLab-ms/access-controller-controller/client"
	"github.com/TheLab-ms/access-controller-controller/keycloak"
)

func TestControllerBasics(t *testing.T) {
	tac := &testAccessController{cards: make(map[int]*client.Card)}
	tus := &testUserStorage{}

	c := &Controller{controller: tac, storage: tus}

	t.Run("empty", func(t *testing.T) {
		changed, err := c.sync(context.Background())
		require.NoError(t, err)
		assert.False(t, changed)
	})

	t.Run("initial creation", func(t *testing.T) {
		tus.users = []*keycloak.AccessUser{{
			UUID:         "592af547-8f68-42d8-8b81-4a5d233b7cce",
			KeyfobNumber: 9001,
		}}

		changed, err := c.sync(context.Background())
		require.NoError(t, err)
		assert.True(t, changed)

		assert.Equal(t, tac.cards, map[int]*client.Card{
			0: {
				ID:     0,
				Number: 9001,
				Name:   "592af5478f6842d88b814a5d233b7cce",
			},
		})
	})

	t.Run("idepotence", func(t *testing.T) {
		changed, err := c.sync(context.Background())
		require.NoError(t, err)
		assert.False(t, changed)
	})

	t.Run("update fob ID for existing user", func(t *testing.T) {
		tus.users = []*keycloak.AccessUser{{
			UUID:         "592af547-8f68-42d8-8b81-4a5d233b7cce",
			KeyfobNumber: 9002,
		}}

		// remove
		changed, err := c.sync(context.Background())
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, tac.cards, map[int]*client.Card{})

		// recreate
		changed, err = c.sync(context.Background())
		require.NoError(t, err)
		assert.True(t, changed)

		// done
		changed, err = c.sync(context.Background())
		require.NoError(t, err)
		assert.False(t, changed)

		// proof
		assert.Equal(t, tac.cards, map[int]*client.Card{
			1: {
				ID:     1,
				Number: 9002,
				Name:   "592af5478f6842d88b814a5d233b7cce",
			},
		})
	})

	t.Run("update UUID ID for existing fob", func(t *testing.T) {
		tus.users = []*keycloak.AccessUser{{
			UUID:         "592af547-8f68-42d8-8b81-4a5d233b7cc2",
			KeyfobNumber: 9002,
		}}

		// remove
		changed, err := c.sync(context.Background())
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, tac.cards, map[int]*client.Card{})

		// recreate
		changed, err = c.sync(context.Background())
		require.NoError(t, err)
		assert.True(t, changed)

		// done
		changed, err = c.sync(context.Background())
		require.NoError(t, err)
		assert.False(t, changed)

		// proof
		assert.Equal(t, tac.cards, map[int]*client.Card{
			2: {
				ID:     2,
				Number: 9002,
				Name:   "592af5478f6842d88b814a5d233b7cc2",
			},
		})
	})

	t.Run("old users aren't touched", func(t *testing.T) {
		tac.cards[100] = &client.Card{
			ID:     100,
			Number: 500,
			Name:   "any name",
		}

		changed, err := c.sync(context.Background())
		require.NoError(t, err)
		assert.False(t, changed)
		assert.NotNil(t, tac.cards[100])
	})

	t.Run("old users are overwritten when badge ID is managed by this process", func(t *testing.T) {
		tac.cards[100] = &client.Card{
			ID:     100,
			Number: 9002, // in use by previous test
			Name:   "any name",
		}

		changed, err := c.sync(context.Background())
		require.NoError(t, err)
		assert.True(t, changed)

		changed, err = c.sync(context.Background())
		require.NoError(t, err)
		assert.False(t, changed)

		assert.Equal(t, tac.cards, map[int]*client.Card{
			2: {
				ID:     2,
				Number: 9002,
				Name:   "592af5478f6842d88b814a5d233b7cc2",
			},
		})
	})

	t.Run("duplicate badge IDs are ignored", func(t *testing.T) {
		tus.users = []*keycloak.AccessUser{
			{
				UUID:         "592af547-8f68-42d8-8b81-4a5d233b7cc3",
				KeyfobNumber: 9002,
			},
			{
				UUID:         "592af547-8f68-42d8-8b81-4a5d233b7cc2",
				KeyfobNumber: 9002,
			},
		}

		changed, err := c.sync(context.Background())
		require.NoError(t, err)
		assert.False(t, changed)

		assert.Equal(t, tac.cards, map[int]*client.Card{
			2: {
				ID:     2,
				Number: 9002,
				Name:   "592af5478f6842d88b814a5d233b7cc2",
			},
		})
	})
}

type testAccessController struct {
	lastID int
	cards  map[int]*client.Card
}

func (t *testAccessController) AddCard(ctx context.Context, num int, name string) error {
	t.cards[t.lastID] = &client.Card{
		ID:     t.lastID,
		Number: num,
		Name:   name,
	}
	t.lastID++
	return nil
}

func (t *testAccessController) RemoveCard(ctx context.Context, id int) error {
	delete(t.cards, id)
	return nil
}

func (t *testAccessController) ListCards(ctx context.Context) ([]*client.Card, error) {
	slice := []*client.Card{}
	for _, item := range t.cards {
		slice = append(slice, item)
	}
	return slice, nil
}

type testUserStorage struct {
	users []*keycloak.AccessUser
}

func (t *testUserStorage) ListUsers(ctx context.Context) ([]*keycloak.AccessUser, error) {
	return t.users, nil
}
func (t *testUserStorage) CreateWebhook(ctx context.Context, webhook *keycloak.Webhook) error {
	return nil
}
func (t *testUserStorage) ListWebhooks(ctx context.Context) ([]*keycloak.Webhook, error) {
	return nil, nil
}
