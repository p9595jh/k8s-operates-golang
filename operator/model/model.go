package model

import (
	"bytes"
	"encoding/json"
)

type CreateJobDTO struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Duration int    `json:"duration"` // in seconds
}

func (j *CreateJobDTO) ToBuffer() *bytes.Buffer {
	data, _ := json.Marshal(j)
	return bytes.NewBuffer(data)
}

type JobData struct {
	ID        string        `json:"id"`
	CreatedAt int64         `json:"createdAt"`
	Data      *CreateJobDTO `json:"data"`
}
