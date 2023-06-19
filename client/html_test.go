package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSwipesListPage(t *testing.T) {
	responseFixture, err := os.Open(filepath.Join("fixtures", "response.html"))
	require.NoError(t, err)
	defer responseFixture.Close()

	actual, err := parseSwipesListPage(responseFixture)
	require.NoError(t, err)

	expectedFixture, err := os.Open(filepath.Join("fixtures", "expected.json"))
	require.NoError(t, err)
	defer expectedFixture.Close()

	expected := []*CardSwipe{}
	err = json.NewDecoder(expectedFixture).Decode(&expected)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}
