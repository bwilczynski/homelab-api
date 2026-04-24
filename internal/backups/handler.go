package backups

import "context"

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
		return nil, err
	}
	return ListBackupTasks200JSONResponse(result), nil
}

func (h *ServerHandler) GetBackupTask(ctx context.Context, request GetBackupTaskRequestObject) (GetBackupTaskResponseObject, error) {
	result, err := h.svc.GetBackupTask(ctx, request.TaskId)
	if err != nil {
		return nil, err
	}
	if result == nil {
		detail := "backup task not found"
		return GetBackupTask404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   "urn:homelab:error:not-found",
				Title:  "Not Found",
				Status: 404,
				Detail: &detail,
			},
		}, nil
	}
	return GetBackupTask200JSONResponse(*result), nil
}
