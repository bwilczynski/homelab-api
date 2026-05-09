package docker

import (
	"github.com/bwilczynski/homelab-api/internal/adapters"
)

// ImagesBackend is the narrow interface for Docker image operations.
type ImagesBackend interface {
	ListDockerImages() (*adapters.DSMDockerImageListResponse, error)
}
