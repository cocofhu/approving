package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// NextScheduleTime computes the next fire time for at|every|cron expressions.
// timezone is an IANA name (e.g. Asia/Shanghai); empty uses the process local zone.
// For "at", expr must be RFC3339 and timezone is ignored (instant is absolute).
func NextScheduleTime(kind, expr, timezone string, from time.Time) (time.Time, error) {
	kind = strings.TrimSpace(kind)
	expr = strings.TrimSpace(expr)
	switch kind {
	case "at":
		t, err := time.Parse(time.RFC3339, expr)
		if err != nil {
			return time.Time{}, fmt.Errorf("at scheduleExpr must be RFC3339: %w", err)
		}
		return t, nil
	case "every":
		d, err := time.ParseDuration(expr)
		if err != nil || d <= 0 {
			return time.Time{}, fmt.Errorf("every scheduleExpr must be duration like 1h, 30m")
		}
		return from.Add(d), nil
	case "cron":
		if len(strings.Fields(expr)) < 5 {
			return time.Time{}, fmt.Errorf("cron scheduleExpr needs 5 fields")
		}
		sched, err := cron.ParseStandard(expr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid cron scheduleExpr: %w", err)
		}
		loc, err := loadScheduleLocation(timezone)
		if err != nil {
			return time.Time{}, err
		}
		next := sched.Next(from.In(loc))
		if next.IsZero() {
			return time.Time{}, fmt.Errorf("cron scheduleExpr has no next run")
		}
		return next, nil
	default:
		return time.Time{}, fmt.Errorf("unknown scheduleKind")
	}
}

func loadScheduleLocation(timezone string) (*time.Location, error) {
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone: %w", err)
	}
	return loc, nil
}

// EscapeLike escapes % and _ for SQL LIKE patterns (parameterized queries).
func EscapeLike(q string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(q)
}
