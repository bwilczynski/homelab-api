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
				Title:  apierrors.TitleInternalServerError,
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
				Title:  apierrors.TitleInternalServerError,
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
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListSystemUtilization200JSONResponse(result), nil
}

// ListSystemUpdates implements StrictServerInterface.
func (h *ServerHandler) ListSystemUpdates(ctx context.Context, request ListSystemUpdatesRequestObject) (ListSystemUpdatesResponseObject, error) {
	result, err := h.svc.ListSystemUpdates(ctx, request.Params.Status, request.Params.Type)
	if err != nil {
		detail := err.Error()
		return ListSystemUpdates500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListSystemUpdates200JSONResponse(result), nil
}

// GetSystemUpdate implements StrictServerInterface.
func (h *ServerHandler) GetSystemUpdate(ctx context.Context, request GetSystemUpdateRequestObject) (GetSystemUpdateResponseObject, error) {
	detail, err := h.svc.GetSystemUpdate(ctx, request.UpdateId)
	if err != nil {
		d := err.Error()
		return GetSystemUpdate500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &d,
			},
		}, nil
	}
	if detail == nil {
		return GetSystemUpdate404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
			},
		}, nil
	}
	return GetSystemUpdate200JSONResponse(*detail), nil
}

// CheckSystemUpdates implements StrictServerInterface.
func (h *ServerHandler) CheckSystemUpdates(ctx context.Context, request CheckSystemUpdatesRequestObject) (CheckSystemUpdatesResponseObject, error) {
	result, err := h.svc.CheckSystemUpdates(ctx)
	if err != nil {
		detail := err.Error()
		return CheckSystemUpdates500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return CheckSystemUpdates200JSONResponse(result), nil
}
