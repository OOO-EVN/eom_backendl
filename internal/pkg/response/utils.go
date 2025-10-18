// handlers/utils.go
package response

import (
	"fmt"
	"strings"
)

func NormalizeSlot(slot string) string {
	slot = strings.ReplaceAll(slot, "–", "-")
	slot = strings.ReplaceAll(slot, "—", "-")
	slot = strings.ReplaceAll(slot, " ", "")
	return slot
}

func FormatDuration(seconds int) string {
	if seconds <= 0 {
		return "0 мин"
	}
	hours := seconds / 3600
	mins := (seconds % 3600) / 60
	if hours > 0 {
		return fmt.Sprintf("%d ч %d мин", hours, mins)
	}
	return fmt.Sprintf("%d мин", mins)
}
