package issues

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const Shown = 50

const (
	ByWorst  = "worst"
	ByNewest = "newest"
	ByOldest = "oldest"
)

func OrderOf(asked string) string {
	if asked == ByNewest || asked == ByOldest {
		return asked
	}
	return ByWorst
}

const (
	foldCeiling = 999999999
	timeCeiling = 9999999999
)

func issueKey(row api.Issue, order string) string {
	if order == ByNewest {
		return strings.Join([]string{
			countDown(secondsOf(seenAt(row.Since)), timeCeiling),
			row.Cluster,
			row.ID,
		}, "\x00")
	}
	if order == ByOldest {
		return strings.Join([]string{
			countUp(secondsOf(seenAt(row.Since)), timeCeiling),
			row.Cluster,
			row.ID,
		}, "\x00")
	}
	return strings.Join([]string{
		strconv.Itoa(severityFatal - severityRank(row.Severity)),
		countDown(row.Folded, foldCeiling),
		countDown(secondsOf(seenAt(row.Since)), timeCeiling),
		row.Cluster,
		row.ID,
	}, "\x00")
}

func countUp(value, ceiling int) string {
	if value < 0 {
		value = 0
	}
	if value > ceiling {
		value = ceiling
	}
	width := len(strconv.Itoa(ceiling))
	return fmt.Sprintf("%0*d", width, value)
}

func countDown(value, ceiling int) string {
	if value < 0 {
		value = 0
	}
	if value > ceiling {
		value = ceiling
	}
	width := len(strconv.Itoa(ceiling))
	return fmt.Sprintf("%0*d", width, ceiling-value)
}

func secondsOf(at time.Time) int {
	if at.IsZero() {
		return 0
	}
	return int(at.Unix())
}

func EncodeCursor(key string) string {
	if key == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(key))
}

func DecodeCursor(cursor string) string {
	if cursor == "" {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return ""
	}
	return string(raw)
}

func Page(rows []api.Issue, after string, limit int, order string) ([]api.Issue, string) {
	out := make([]api.Issue, 0, min(limit, len(rows)))
	last := ""
	for _, row := range rows {
		key := issueKey(row, order)
		if key <= after {
			continue
		}
		if len(out) == limit {
			return out, EncodeCursor(last)
		}
		out = append(out, row)
		last = key
	}
	return out, ""
}

func PageSize(shown int) int {
	if shown <= 0 {
		return Shown
	}
	return shown
}
