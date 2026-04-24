package system

import (
	"context"

	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// ServerHandler implements StrictServerInterface by delegating to the Service.
type ServerHandler struct {
	svc *Service
}

// NewHandler creates a new ServerHandler.
func NewHandler(svc *Service) *ServerHandler {
	return &ServerHandler{svc: svc}
}

// GetSystemHealth implements StrictServerInterface.
func (h *ServerHandler) GetSystemHealth(ctx context.Context, request GetSystemHealthRequestObject) (GetSystemHealthResponseObject, error) {
	health, err := h.svc.GetSystemHealth(ctx)
	if err != nil {
		detail := err.Error()
		return GetSystemHealth500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  "Internal Server Error",
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return GetSystemHealth200JSONResponse(health), nil
}

// ListSystemInfo implements StrictServerInterface.
func (h *ServerHandler) ListSystemInfo(ctx context.Context, request ListSystemInfoRequestObject) (ListSystemInfoResponseObject, error) {
	result, err := h.svc.ListSystemInfo(ctx, request.Params.Device)
	if err != nil {
		detail := err.Error()
		return ListSystemInfo500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  "Internal Server Error",
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListSystemInfo200JSONResponse(result), nil
}

// ListSystemUtilization implements StrictServerInterface.
func (h *ServerHandler) ListSystemUtilization(ctx context.Context, request ListSystemUtilizationRequestObject) (ListSystemUtilizationResponseObject, error) {
	result, err := h.svc.ListSystemUtilization(ctx, request.Params.Device)
	if err != nil {
		detail := err.Error()
		return ListSystemUtilization500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  "Internal Server Error",
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListSystemUtilization200JSONResponse(result), nil
}
