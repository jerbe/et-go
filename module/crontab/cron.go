package crontab

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type cronSchedule struct {
	minutes  [60]bool
	hours    [24]bool
	days     [32]bool
	months   [13]bool
	weekdays [7]bool
}

func parseCron(expression string) (*cronSchedule, error) {
	parts := strings.Fields(expression)
	if len(parts) != 5 {
		return nil, ErrInvalidCronExpression
	}
	minutes, err := parseField(parts[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	hours, err := parseField(parts[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	days, err := parseField(parts[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day: %w", err)
	}
	months, err := parseField(parts[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	weekdays, err := parseField(parts[4], 0, 7)
	if err != nil {
		return nil, fmt.Errorf("weekday: %w", err)
	}
	schedule := &cronSchedule{}
	for i := range minutes {
		schedule.minutes[i] = minutes[i]
	}
	for i := range hours {
		schedule.hours[i] = hours[i]
	}
	for i := range days {
		schedule.days[i] = days[i]
	}
	for i := range months {
		schedule.months[i] = months[i]
	}
	for i := 0; i < len(weekdays); i++ {
		if weekdays[i] {
			index := i
			if index == 7 {
				index = 0
			}
			if index >= len(schedule.weekdays) {
				continue
			}
			schedule.weekdays[index] = true
		}
	}
	return schedule, nil
}

func (s *cronSchedule) Match(t time.Time) bool {
	return s.minutes[t.Minute()] &&
		s.hours[t.Hour()] &&
		s.days[t.Day()] &&
		s.months[int(t.Month())] &&
		s.weekdays[int(t.Weekday())]
}

func parseField(value string, min, max int) ([]bool, error) {
	result := make([]bool, max+1)
	tokens := strings.Split(value, ",")
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		step := 1
		if strings.Contains(token, "/") {
			parts := strings.Split(token, "/")
			if len(parts) != 2 {
				return nil, ErrInvalidCronExpression
			}
			token = parts[0]
			s, err := strconv.Atoi(parts[1])
			if err != nil || s <= 0 {
				return nil, ErrInvalidCronExpression
			}
			step = s
		}
		switch {
		case token == "*":
			fillRange(result, min, max, step)
		case strings.Contains(token, "-"):
			bounds := strings.Split(token, "-")
			if len(bounds) != 2 {
				return nil, ErrInvalidCronExpression
			}
			start, err1 := strconv.Atoi(bounds[0])
			end, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil || start > end || start < min || end > max {
				return nil, ErrInvalidCronExpression
			}
			fillRange(result, start, end, step)
		default:
			num, err := strconv.Atoi(token)
			if err != nil || num < min || num > max {
				return nil, ErrInvalidCronExpression
			}
			result[num] = true
		}
	}
	return result, nil
}

func fillRange(arr []bool, start, end, step int) {
	for i := start; i <= end; i += step {
		if i >= 0 && i < len(arr) {
			arr[i] = true
		}
	}
}
