package dto

type CreateJobDTO struct {
	Name     string `json:"name"`
	Duration int    `json:"duration"` // in seconds
}

type CreateJobResp struct {
	Name      string `json:"name"`
	StartedAt string `json:"startedAt"`
}

type FindJobResp struct {
	Name      string `json:"name"`
	Duration  int    `json:"duration"` // in seconds
	StartedAt string `json:"startedAt"`
}
