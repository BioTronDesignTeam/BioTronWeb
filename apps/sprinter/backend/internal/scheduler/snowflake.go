package scheduler

import (
	"strconv"
	"time"
)

// discordEpochMillis is 2015-01-01T00:00:00Z in Unix milliseconds, the epoch
// Discord's snowflake ids count from.
const discordEpochMillis = 1420070400000

// snowflakeAfter builds the smallest Discord snowflake id that could belong
// to a message sent at or after t. Passed as ChannelMessages' afterID, it
// turns "every message since t" into a single Discord query, because a
// snowflake id encodes its creation time in its high bits and Discord's API
// takes a message id, not a timestamp, for that boundary.
func snowflakeAfter(t time.Time) string {
	millis := t.UnixMilli() - discordEpochMillis
	if millis < 0 {
		millis = 0
	}
	return strconv.FormatUint(uint64(millis)<<22, 10)
}
