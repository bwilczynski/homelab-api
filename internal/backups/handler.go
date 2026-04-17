package backups

import "context"

type ServerHandler struct {
	svc *Service
}

func NewHandler(svc *Service) *ServerHandler {
	return &ServerHandler{svc: svc}
}

func (h *ServerHandler) ListBackupTasks(ctx context.Context, request ListBackupTasksRequestObject) (ListBackupTasksResponseObject, error) {
	return nil, nil
}

func (h *ServerHandler) GetBackupTask(ctx context.Context, request GetBackupTaskRequestObject) (GetBackupTaskResponseObject, error) {
	return nil, nil
}
