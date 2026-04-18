package storage

import "context"

// ServerHandler implements the generated StrictServerInterface for storage.
type ServerHandler struct {
	svc *Service
}

// NewHandler creates a new storage handler.
func NewHandler(svc *Service) *ServerHandler {
	return &ServerHandler{svc: svc}
}

func (h *ServerHandler) ListStorageVolumes(ctx context.Context, request ListStorageVolumesRequestObject) (ListStorageVolumesResponseObject, error) {
	result, err := h.svc.ListStorageVolumes(ctx, request.Params.Device)
	if err != nil {
		return nil, err
	}
	return ListStorageVolumes200JSONResponse(result), nil
}

func (h *ServerHandler) GetStorageVolume(ctx context.Context, request GetStorageVolumeRequestObject) (GetStorageVolumeResponseObject, error) {
	result, err := h.svc.GetStorageVolume(ctx, request.VolumeId)
	if err != nil {
		return nil, err
	}
	if result == nil {
		detail := "volume not found"
		return GetStorageVolume404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   "urn:homelab:error:not-found",
				Title:  "Not Found",
				Status: 404,
				Detail: &detail,
			},
		}, nil
	}
	return GetStorageVolume200JSONResponse(*result), nil
}
