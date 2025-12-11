package service

import (
	"context"
	"net/http"
	"operatable/cerror"
	"operatable/dto"
	"operatable/model"
	"time"
)

type JobService struct {
	job *model.Job
}

func NewJobService() *JobService {
	return &JobService{}
}

func (s *JobService) Find(ctx context.Context) (*dto.FindJobResp, error) {
	if s.job == nil {
		return nil, cerror.New4(http.StatusNotFound, "no job found")
	}

	return &dto.FindJobResp{
		Name:      s.job.Name,
		Duration:  int(s.job.Duration.Seconds()),
		StartedAt: s.job.StartedAt.Format(time.RFC3339),
	}, nil
}

func (s *JobService) Create(ctx context.Context, jobDTO *dto.CreateJobDTO) (*dto.CreateJobResp, error) {
	if s.job != nil {
		return nil, cerror.New4(http.StatusTooManyRequests, "job already running")
	}

	s.job = &model.Job{
		Name:      jobDTO.Name,
		Duration:  time.Duration(jobDTO.Duration) * time.Second,
		StartedAt: time.Now(),
	}

	go func() {
		time.Sleep(s.job.Duration)
		s.job = nil
	}()

	return &dto.CreateJobResp{
		Name:      s.job.Name,
		StartedAt: s.job.StartedAt.Format(time.RFC3339),
	}, nil
}

func (s *JobService) Delete(ctx context.Context) error {
	if s.job == nil {
		return cerror.New4(http.StatusNotFound, "no job to delete")
	}

	s.job = nil
	return nil
}
