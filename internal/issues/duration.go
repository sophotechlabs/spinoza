package issues

import (
	"strconv"
	"time"
)

func durationLabel(elapsed time.Duration) string {
	if elapsed < 0 {
		elapsed = 0
	}
	seconds := int64(elapsed / time.Second)
	if seconds < 60 {
		return strconv.FormatInt(seconds, 10) + "s"
	}
	minutes := seconds / 60
	if minutes < 60 {
		return strconv.FormatInt(minutes, 10) + "m"
	}
	hours := minutes / 60
	if hours < 24 {
		return strconv.FormatInt(hours, 10) + "h"
	}
	days := hours / 24
	return strconv.FormatInt(days, 10) + "d"
}
