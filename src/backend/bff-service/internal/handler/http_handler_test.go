package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractBearer(t *testing.T) {
	assert.Equal(t, "token", extractBearer("Bearer token"))
	assert.Equal(t, "token", extractBearer("bearer token"))
	assert.Equal(t, "", extractBearer("token"))
	assert.Equal(t, "", extractBearer("Bearer"))
}
