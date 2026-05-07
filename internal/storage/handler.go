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

func (h *ServerHandler) ListBackups(ctx context.Context, request ListBackupsRequestObject) (ListBackupsResponseObject, error) {
	result, err := h.svc.ListBackupTasks(ctx, request.Params.Device)
	if err != nil {
		detail := err.Error()
		return ListBackups500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListBackups200JSONResponse(result), nil
}

func (h *ServerHandler) GetBackup(ctx context.Context, request GetBackupRequestObject) (GetBackupResponseObject, error) {
	result, err := h.svc.GetBackupTask(ctx, request.BackupId)
	if err != nil {
		detail := err.Error()
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetBackup404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNNotFound,
					Title:  apierrors.TitleNotFound,
					Status: 404,
					Detail: &detail,
				},
			}, nil
		}
		return GetBackup500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	if result == nil {
		detail := "backup task not found"
		return GetBackup404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
				Detail: &detail,
			},
		}, nil
	}
	return GetBackup200JSONResponse(*result), nil
}

func (h *ServerHandler) ListStorageVolumes(ctx context.Context, request ListStorageVolumesRequestObject) (ListStorageVolumesResponseObject, error) {
	result, err := h.svc.ListStorageVolumes(ctx, request.Params.Device)
	if err != nil {
		detail := err.Error()
		return ListStorageVolumes500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListStorageVolumes200JSONResponse(result), nil
}

func (h *ServerHandler) GetStorageVolume(ctx context.Context, request GetStorageVolumeRequestObject) (GetStorageVolumeResponseObject, error) {
	result, err := h.svc.GetStorageVolume(ctx, request.VolumeId)
	if err != nil {
		detail := err.Error()
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetStorageVolume404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNNotFound,
					Title:  apierrors.TitleNotFound,
					Status: 404,
					Detail: &detail,
				},
			}, nil
		}
		return GetStorageVolume500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	if result == nil {
		detail := "volume not found"
		return GetStorageVolume404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
				Detail: &detail,
			},
		}, nil
	}
	return GetStorageVolume200JSONResponse(*result), nil
}
