package system

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// systemUpdateDetailResponse is a hand-written response wrapper for GetSystemUpdate 200.
// GetSystemUpdate200JSONResponse is defined as `type T SystemUpdateDetail`, which loses
// the custom MarshalJSON on SystemUpdateDetail, causing {} to be serialised. This type
// holds the original SystemUpdateDetail and delegates encoding to it directly.
type systemUpdateDetailResponse struct{ detail SystemUpdateDetail }

func (r systemUpdateDetailResponse) VisitGetSystemUpdateResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(r.detail)
}

// ServerHandler implements StrictServerInterface by delegating to the Service.
type ServerHandler struct {
	svc *Service
}

// NewHandler creates a new ServerHandler.
func NewHandler(svc *Service) *ServerHandler {
	return &ServerHandler{svc: svc}
}

func internalServerError(detail string) InternalServerErrorApplicationProblemPlusJSONResponse {
	resp := InternalServerErrorApplicationProblemPlusJSONResponse{
		Type:   apierrors.URNInternalServerError,
		Title:  apierrors.TitleInternalServerError,
		Status: 500,
	}
	if detail != "" {
		resp.Detail = &detail
	}
	return resp
}

func notFound(detail string) NotFoundApplicationProblemPlusJSONResponse {
	resp := NotFoundApplicationProblemPlusJSONResponse{
		Type:   apierrors.URNNotFound,
		Title:  apierrors.TitleNotFound,
		Status: 404,
	}
	if detail != "" {
		resp.Detail = &detail
	}
	return resp
}

func badRequest(detail string) BadRequestApplicationProblemPlusJSONResponse {
	resp := BadRequestApplicationProblemPlusJSONResponse{
		Type:   apierrors.URNBadRequest,
		Title:  apierrors.TitleBadRequest,
		Status: 400,
	}
	if detail != "" {
		resp.Detail = &detail
	}
	return resp
}

// GetSystemHealth implements StrictServerInterface.
func (h *ServerHandler) GetSystemHealth(ctx context.Context, request GetSystemHealthRequestObject) (GetSystemHealthResponseObject, error) {
	health, err := h.svc.GetSystemHealth(ctx)
	if err != nil {
		return GetSystemHealth500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return GetSystemHealth200JSONResponse(health), nil
}

// ListSystemInfo implements StrictServerInterface.
func (h *ServerHandler) ListSystemInfo(ctx context.Context, request ListSystemInfoRequestObject) (ListSystemInfoResponseObject, error) {
	result, err := h.svc.ListSystemInfo(ctx, request.Params.Device)
	if err != nil {
		return ListSystemInfo500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListSystemInfo200JSONResponse(result), nil
}

// ListSystemUtilization implements StrictServerInterface.
func (h *ServerHandler) ListSystemUtilization(ctx context.Context, request ListSystemUtilizationRequestObject) (ListSystemUtilizationResponseObject, error) {
	result, err := h.svc.ListSystemUtilization(ctx, request.Params.Device)
	if err != nil {
		return ListSystemUtilization500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListSystemUtilization200JSONResponse(result), nil
}

// ListSystemUpdates implements StrictServerInterface.
func (h *ServerHandler) ListSystemUpdates(ctx context.Context, request ListSystemUpdatesRequestObject) (ListSystemUpdatesResponseObject, error) {
	if s := request.Params.Status; s != nil && !s.Valid() {
		return ListSystemUpdates400ApplicationProblemPlusJSONResponse{badRequest("Invalid value for parameter 'status': must be one of upToDate, updateAvailable, unknown.")}, nil
	}
	if t := request.Params.Type; t != nil && !t.Valid() {
		return ListSystemUpdates400ApplicationProblemPlusJSONResponse{badRequest("Invalid value for parameter 'type': must be one of container.")}, nil
	}
	result, err := h.svc.ListSystemUpdates(ctx, request.Params.Status, request.Params.Type)
	if err != nil {
		return ListSystemUpdates500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListSystemUpdates200JSONResponse(result), nil
}

// GetSystemUpdate implements StrictServerInterface.
func (h *ServerHandler) GetSystemUpdate(ctx context.Context, request GetSystemUpdateRequestObject) (GetSystemUpdateResponseObject, error) {
	detail, err := h.svc.GetSystemUpdate(ctx, request.UpdateId)
	if err != nil {
		return GetSystemUpdate500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	if detail == nil {
		return GetSystemUpdate404ApplicationProblemPlusJSONResponse{notFound("")}, nil
	}
	return systemUpdateDetailResponse{*detail}, nil
}

// CheckSystemUpdates implements StrictServerInterface.
func (h *ServerHandler) CheckSystemUpdates(ctx context.Context, request CheckSystemUpdatesRequestObject) (CheckSystemUpdatesResponseObject, error) {
	result, err := h.svc.CheckSystemUpdates(ctx)
	if err != nil {
		return CheckSystemUpdates500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return CheckSystemUpdates200JSONResponse(result), nil
}
