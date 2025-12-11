package model

import "time"

type Job struct {
	Name      string
	Duration  time.Duration
	StartedAt time.Time
}
