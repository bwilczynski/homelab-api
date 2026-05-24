package storage

import (
	"context"
	"errors"

	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// ServerHandler implements the generated StrictServerInterface for storage.
type ServerHandler struct {
	svc *Service
}

// NewHandler creates a new storage handler.
func NewHandler(svc *Service) *ServerHandler {
	return &ServerHandler{svc: svc}
}

func internalServerError(detail string) InternalServerErrorApplicationProblemPlusJSONResponse {
	return InternalServerErrorApplicationProblemPlusJSONResponse{
		Type:   apierrors.URNInternalServerError,
		Title:  apierrors.TitleInternalServerError,
		Status: 500,
		Detail: &detail,
	}
}

func notFound(detail string) NotFoundApplicationProblemPlusJSONResponse {
	return NotFoundApplicationProblemPlusJSONResponse{
		Type:   apierrors.URNNotFound,
		Title:  apierrors.TitleNotFound,
		Status: 404,
		Detail: &detail,
	}
}

func (h *ServerHandler) ListBackups(ctx context.Context, request ListBackupsRequestObject) (ListBackupsResponseObject, error) {
	result, err := h.svc.ListBackupTasks(ctx, request.Params.Device)
	if err != nil {
		return ListBackups500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListBackups200JSONResponse(result), nil
}

func (h *ServerHandler) GetBackup(ctx context.Context, request GetBackupRequestObject) (GetBackupResponseObject, error) {
	result, err := h.svc.GetBackupTask(ctx, request.BackupId)
	if err != nil {
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetBackup404ApplicationProblemPlusJSONResponse{notFound(err.Error())}, nil
		}
		return GetBackup500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	if result == nil {
		return GetBackup404ApplicationProblemPlusJSONResponse{notFound("backup task not found")}, nil
	}
	return GetBackup200JSONResponse(*result), nil
}

func (h *ServerHandler) ListStorageVolumes(ctx context.Context, request ListStorageVolumesRequestObject) (ListStorageVolumesResponseObject, error) {
	result, err := h.svc.ListStorageVolumes(ctx, request.Params.Device)
	if err != nil {
		return ListStorageVolumes500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListStorageVolumes200JSONResponse(result), nil
}

func (h *ServerHandler) GetStorageVolume(ctx context.Context, request GetStorageVolumeRequestObject) (GetStorageVolumeResponseObject, error) {
	result, err := h.svc.GetStorageVolume(ctx, request.VolumeId)
	if err != nil {
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetStorageVolume404ApplicationProblemPlusJSONResponse{notFound(err.Error())}, nil
		}
		return GetStorageVolume500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	if result == nil {
		return GetStorageVolume404ApplicationProblemPlusJSONResponse{notFound("volume not found")}, nil
	}
	return GetStorageVolume200JSONResponse(*result), nil
}
