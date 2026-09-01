package trixorm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPublishWithContextReturnsRedisErrors(t *testing.T) {
	registry := &Registry{}
	registry.RegisterRedis("localhost:6399", "", 15)
	validatedRegistry, cleanup, err := registry.Validate()
	require.NoError(t, err)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = validatedRegistry.CreateEngine().GetRedis().PublishWithContext(ctx, "updates", "payload")
	require.Error(t, err)
}
