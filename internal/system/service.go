package system

import (
	"context"
	"time"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) GetSystemHealth(ctx context.Context) (Health, error) {
	return Health{
		Status:     Healthy,
		CheckedAt:  time.Now().UTC(),
		Components: []ComponentHealth{},
	}, nil
}
