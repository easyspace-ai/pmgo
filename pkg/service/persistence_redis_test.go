package service

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/c9s/bbgo/pkg/fixedpoint"
)

func TestRedisPersistentService(t *testing.T) {
	// 需要本地/CI 提供可用 Redis（默认 127.0.0.1:6379）。
	// 云端执行环境通常没有 Redis，因此默认跳过，避免因环境差异导致测试失败。
	if os.Getenv("BBGO_REDIS_TEST") == "" {
		t.Skip("skip redis persistence test; set BBGO_REDIS_TEST=1 to enable")
	}

	redisService := NewRedisPersistenceService(&RedisPersistenceConfig{
		Host: "127.0.0.1",
		Port: "6379",
		DB:   0,
	})
	assert.NotNil(t, redisService)

	store := redisService.NewStore("bbgo", "test")
	assert.NotNil(t, store)

	err := store.Reset()
	assert.NoError(t, err)

	var fp fixedpoint.Value
	err = store.Load(fp)
	assert.Error(t, err)
	assert.EqualError(t, ErrPersistenceNotExists, err.Error())

	fp = fixedpoint.NewFromFloat(3.1415)
	err = store.Save(&fp)
	assert.NoError(t, err, "should store value without error")

	var fp2 fixedpoint.Value
	err = store.Load(&fp2)
	assert.NoError(t, err, "should load value without error")
	assert.Equal(t, fp, fp2)

	err = store.Reset()
	assert.NoError(t, err)
}
