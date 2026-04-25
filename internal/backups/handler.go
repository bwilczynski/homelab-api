package backups

import (
	"context"
	"errors"

	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// ServerHandler implements the generated StrictServerInterface for backups.
type ServerHandler struct {
	svc *Service
}

// NewHandler creates a new backup handler.
func NewHandler(svc *Service) *ServerHandler {
	return &ServerHandler{svc: svc}
}

func (h *ServerHandler) ListBackupTasks(ctx context.Context, request ListBackupTasksRequestObject) (ListBackupTasksResponseObject, error) {
	result, err := h.svc.ListBackupTasks(ctx, request.Params.Device)
	if err != nil {
		detail := err.Error()
		return ListBackupTasks500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListBackupTasks200JSONResponse(result), nil
}

func (h *ServerHandler) GetBackupTask(ctx context.Context, request GetBackupTaskRequestObject) (GetBackupTaskResponseObject, error) {
	result, err := h.svc.GetBackupTask(ctx, request.TaskId)
	if err != nil {
		detail := err.Error()
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetBackupTask404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNNotFound,
					Title:  apierrors.TitleNotFound,
					Status: 404,
					Detail: &detail,
				},
			}, nil
		}
		return GetBackupTask500ApplicationProblemPlusJSONResponse{
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
		return GetBackupTask404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
				Detail: &detail,
			},
		}, nil
	}
	return GetBackupTask200JSONResponse(*result), nil
}
