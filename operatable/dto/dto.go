package dto

type CreateJobDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Duration int    `json:"duration"` // in seconds
}

type CreateJobResp struct {
	StartedAt string `json:"startedAt"`
}

type FindJobResp struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Duration  int    `json:"duration"` // in seconds
	StartedAt string `json:"startedAt"`
}
