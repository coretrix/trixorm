package tools

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExtraEscapeSQLParamScenarios(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain text", "hello", "'hello'"},
		{"single quote", "Bob's", "'Bob\\'s'"},
		{"double quote", `say "hi"`, `'say \"hi\"'`},
		{"backslash", `a\b`, `'a\\b'`},
		{"new line", "a\nb", "'a\\nb'"},
		{"carriage return", "a\rb", "'a\\rb'"},
		{"zero byte", "a\x00b", "'a\\0b'"},
		{"ctrl z", "a\x1ab", "'a\\Zb'"},
		{"mixed escaping", "a\x00\n\r\\'\"\x1a", "'a\\0\\n\\r\\\\\\'\\\"\\Z'"},
		{"empty string", "", "''"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, EscapeSQLParam(test.input))
		})
	}
}

func TestExtraRedisStreamIDToSinceScenarios(t *testing.T) {
	now := time.Unix(10, 500*int64(time.Millisecond))

	t.Run("empty id returns zero duration", func(t *testing.T) {
		duration, parsed := idToSince("", now)

		assert.Equal(t, time.Duration(0), duration)
		assert.NotZero(t, parsed)
	})

	t.Run("zero id returns zero duration", func(t *testing.T) {
		duration, parsed := idToSince("0-0", now)

		assert.Equal(t, time.Duration(0), duration)
		assert.NotZero(t, parsed)
	})

	t.Run("older id returns positive duration and parsed time", func(t *testing.T) {
		duration, parsed := idToSince("9000-1", now)

		assert.Equal(t, time.Unix(9, 0), parsed)
		assert.Equal(t, 1500*time.Millisecond, duration)
	})

	t.Run("future id is clamped to zero duration", func(t *testing.T) {
		duration, parsed := idToSince("12000-1", now)

		assert.Equal(t, time.Unix(12, 0), parsed)
		assert.Equal(t, time.Duration(0), duration)
	})

	t.Run("malformed id is parsed as unix zero", func(t *testing.T) {
		duration, parsed := idToSince("not-a-number", now)

		assert.Equal(t, time.Unix(0, 0), parsed)
		assert.Equal(t, now.Sub(time.Unix(0, 0)), duration)
	})
}
