package logs

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

const browserStore = "../../frontend/src/store/logs.ts"

var browserBuffer = regexp.MustCompile(`export const MAX_LOG_LINES = (\d+);`)

func TestTheBrowserKeepsEveryLineAStreamOpensWith(t *testing.T) {
	source, err := os.ReadFile(browserStore)
	if err != nil {
		t.Fatalf("read %s: %v", browserStore, err)
	}
	found := browserBuffer.FindStringSubmatch(string(source))
	if found == nil {
		t.Fatalf("%s no longer declares MAX_LOG_LINES", browserStore)
	}
	kept, err := strconv.Atoi(found[1])
	if err != nil {
		t.Fatalf("MAX_LOG_LINES = %q: %v", found[1], err)
	}
	if kept < tailBudget {
		t.Fatalf("the browser keeps %d lines and a stream opens with up to %d", kept, tailBudget)
	}
}
