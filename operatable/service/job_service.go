package service

import (
	"context"
	"fmt"
	"net/http"
	"operatable/cerror"
	"operatable/config"
	"operatable/dto"
	"operatable/model"
	"os"
	"time"

	"github.com/rs/zerolog/log"
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
		ID:        jobDTO.ID,
		Name:      jobDTO.Name,
		Duration:  time.Duration(jobDTO.Duration) * time.Second,
		StartedAt: time.Now(),
	}

	go func() {
		time.Sleep(s.job.Duration)

		req, err := http.NewRequest(
			http.MethodDelete,
			fmt.Sprintf(
				"%s/%s/%s",
				config.Get().System.DoneCallbackURL,
				os.Getenv("POD_NAME"),
				s.job.ID,
			),
			nil,
		)
		if err != nil {
			log.Error().Err(err).Msg("failed to create done callback request")
			return
		}

		client := &http.Client{}
		_, err = client.Do(req)
		if err != nil {
			log.Error().Err(err).Msg("failed to send done callback request")
			return
		}

		s.job = nil
	}()

	return &dto.CreateJobResp{
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
