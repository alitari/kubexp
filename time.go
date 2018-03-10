package kubeExplorer

import (
	"fmt"
	"time"
)

func time2age(st time.Time) string {
	year, month, day, hour, min, sec := timeDiff(time.Now(), st)
	if year > 0 {
		return fmt.Sprintf("%dy%dm", year, month)
	} else if month > 0 {
		return fmt.Sprintf("%dm%dd", month, day)
	} else if day > 0 {
		return fmt.Sprintf("%dd%dh", day, hour)
	} else if hour > 0 {
		return fmt.Sprintf("%dh%dm", hour, min)
	} else if min > 0 {
		return fmt.Sprintf("%dm%ds", min, sec)
	}
	return fmt.Sprintf("%ds", sec)
}

func totime(s string) time.Time {
	layout := "2006-01-02T15:04:05Z"
	t, _ := time.Parse(layout, s)
	return t
}

func timeDiff(a, b time.Time) (year, month, day, hour, min, sec int) {
	if a.Location() != b.Location() {
		b = b.In(a.Location())
	}
	if a.After(b) {
		a, b = b, a
	}
	y1, M1, d1 := a.Date()
	y2, M2, d2 := b.Date()

	h1, m1, s1 := a.Clock()
	h2, m2, s2 := b.Clock()

	year = int(y2 - y1)
	month = int(M2 - M1)
	day = int(d2 - d1)
	hour = int(h2 - h1)
	min = int(m2 - m1)
	sec = int(s2 - s1)

	// Normalize negative values
	if sec < 0 {
		sec += 60
		min--
	}
	if min < 0 {
		min += 60
		hour--
	}
	if hour < 0 {
		hour += 24
		day--
	}
	if day < 0 {
		// days in month:
		t := time.Date(y1, M1, 32, 0, 0, 0, 0, time.UTC)
		day += 32 - t.Day()
		month--
	}
	if month < 0 {
		month += 12
		year--
	}

	return
}
