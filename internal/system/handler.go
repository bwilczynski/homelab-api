package system

import "context"

type ServerHandler struct {
	svc *Service
}

func NewHandler(svc *Service) *ServerHandler {
	return &ServerHandler{svc: svc}
}

func (h *ServerHandler) GetSystemHealth(ctx context.Context, request GetSystemHealthRequestObject) (GetSystemHealthResponseObject, error) {
	health, err := h.svc.GetSystemHealth(ctx)
	if err != nil {
		return nil, err
	}
	return GetSystemHealth200JSONResponse(health), nil
}

func (h *ServerHandler) ListSystemInfo(ctx context.Context, request ListSystemInfoRequestObject) (ListSystemInfoResponseObject, error) {
	return nil, nil
}

func (h *ServerHandler) ListSystemUtilization(ctx context.Context, request ListSystemUtilizationRequestObject) (ListSystemUtilizationResponseObject, error) {
	return nil, nil
}
