package storage

import "context"

type ServerHandler struct {
	svc *Service
}

func NewHandler(svc *Service) *ServerHandler {
	return &ServerHandler{svc: svc}
}

func (h *ServerHandler) ListStorageVolumes(ctx context.Context, request ListStorageVolumesRequestObject) (ListStorageVolumesResponseObject, error) {
	return nil, nil
}

func (h *ServerHandler) GetStorageVolume(ctx context.Context, request GetStorageVolumeRequestObject) (GetStorageVolumeResponseObject, error) {
	return nil, nil
}
