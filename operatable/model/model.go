package model

import "time"

type Job struct {
	ID        string
	Name      string
	Duration  time.Duration
	StartedAt time.Time
}
