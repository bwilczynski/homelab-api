package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwilczynski/homelab-api/internal/adapters"
	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// ImagesBackend is the narrow interface for Docker image operations.
type ImagesBackend interface {
	ListDockerImages() (*adapters.DSMDockerImageListResponse, error)
}

// imageShortID strips the "sha256:" prefix and returns the first 12 hex characters.
func imageShortID(fullID string) string {
	s := strings.TrimPrefix(fullID, "sha256:")
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// ListImages returns all Docker images from all backends.
func (s *Service) ListImages(ctx context.Context, device *string) (DockerImageList, error) {
	var items []DockerImage
	for _, db := range s.backends {
		if device != nil && *device != db.device {
			continue
		}
		if !db.backend.SupportsContainers() {
			continue
		}
		if s.monitor != nil && !s.monitor.Available(db.device) {
			continue
		}
		raw, err := db.backend.ListDockerImages()
		if err != nil {
			return DockerImageList{}, fmt.Errorf("list docker images from %s: %w", db.device, err)
		}
		for _, img := range raw.Images {
			items = append(items, mapDockerImage(db.device, img))
		}
	}
	if items == nil {
		items = []DockerImage{}
	}
	return DockerImageList{Items: items}, nil
}

// GetImage returns a single Docker image by composite ID "{device}.{shortID}".
func (s *Service) GetImage(ctx context.Context, imageID string) (*DockerImageDetail, error) {
	device, shortID, err := parseDockerID(imageID)
	if err != nil {
		return nil, err
	}
	backend, err := s.findBackend(device)
	if err != nil {
		return nil, err
	}
	raw, err := backend.ListDockerImages()
	if err != nil {
		return nil, fmt.Errorf("list docker images: %w", err)
	}
	for _, img := range raw.Images {
		if strings.HasPrefix(img.ID, "sha256:"+shortID) {
			detail := mapDockerImageDetail(device, img)
			return &detail, nil
		}
	}
	return nil, fmt.Errorf("image %q not found: %w", imageID, apierrors.ErrNotFound)
}

func mapDockerImage(device string, img adapters.DSMDockerImageItem) DockerImage {
	tags := img.Tags
	if tags == nil {
		tags = []string{}
	}
	return DockerImage{
		Id:         fmt.Sprintf("%s.%s", device, imageShortID(img.ID)),
		Device:     device,
		Repository: img.Repository,
		Size:       img.Size,
		Tags:       tags,
	}
}

func mapDockerImageDetail(device string, img adapters.DSMDockerImageItem) DockerImageDetail {
	tags := img.Tags
	if tags == nil {
		tags = []string{}
	}
	detail := DockerImageDetail{
		Id:          fmt.Sprintf("%s.%s", device, imageShortID(img.ID)),
		Device:      device,
		Repository:  img.Repository,
		Size:        img.Size,
		VirtualSize: img.VirtualSize,
		Tags:        tags,
		Created:     time.Unix(img.Created, 0).UTC(),
	}
	if img.Description != "" {
		d := img.Description
		detail.Description = &d
	}
	return detail
}
