package network

import (
	"context"
	"errors"
	"net/http"

	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// networkClientDetailResponse is a custom 200 response for GetNetworkClient that
// correctly serializes NetworkClientDetail's anyOf union. The generated
// GetNetworkClient200JSONResponse is a new type (not an alias), so it does not
// inherit NetworkClientDetail.MarshalJSON — encoding it produces {}.
// This type calls MarshalJSON explicitly.
type networkClientDetailResponse struct {
	detail NetworkClientDetail
}

func (r networkClientDetailResponse) VisitGetNetworkClientResponse(w http.ResponseWriter) error {
	b, err := r.detail.MarshalJSON()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(b)
	return err
}

// networkDeviceDetailResponse is a custom 200 response for GetNetworkDevice that
// correctly serializes NetworkDeviceDetail's anyOf union. The generated
// GetNetworkDevice200JSONResponse is a new type (not an alias), so it does not
// inherit NetworkDeviceDetail.MarshalJSON — encoding it produces {}.
// This type calls MarshalJSON explicitly.
type networkDeviceDetailResponse struct {
	detail NetworkDeviceDetail
}

func (r networkDeviceDetailResponse) VisitGetNetworkDeviceResponse(w http.ResponseWriter) error {
	b, err := r.detail.MarshalJSON()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(b)
	return err
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

func badRequest(detail string) BadRequestApplicationProblemPlusJSONResponse {
	return BadRequestApplicationProblemPlusJSONResponse{
		Type:   apierrors.URNBadRequest,
		Title:  apierrors.TitleBadRequest,
		Status: 400,
		Detail: &detail,
	}
}

// ListNetworkDevices implements StrictServerInterface.
func (h *ServerHandler) ListNetworkDevices(ctx context.Context, request ListNetworkDevicesRequestObject) (ListNetworkDevicesResponseObject, error) {
	result, err := h.svc.ListDevices(ctx)
	if err != nil {
		return ListNetworkDevices500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListNetworkDevices200JSONResponse(result), nil
}

// GetNetworkDevice implements StrictServerInterface.
func (h *ServerHandler) GetNetworkDevice(ctx context.Context, request GetNetworkDeviceRequestObject) (GetNetworkDeviceResponseObject, error) {
	detail, found, err := h.svc.GetDevice(ctx, request.DeviceId)
	if err != nil {
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetNetworkDevice404ApplicationProblemPlusJSONResponse{notFound(err.Error())}, nil
		}
		return GetNetworkDevice500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	if !found {
		return GetNetworkDevice404ApplicationProblemPlusJSONResponse{notFound("Network device not found: " + request.DeviceId)}, nil
	}
	return networkDeviceDetailResponse{detail: detail}, nil
}

// ListNetworkClients implements StrictServerInterface.
func (h *ServerHandler) ListNetworkClients(ctx context.Context, request ListNetworkClientsRequestObject) (ListNetworkClientsResponseObject, error) {
	var status string
	if request.Params.Status != nil {
		if !request.Params.Status.Valid() {
			return ListNetworkClients400ApplicationProblemPlusJSONResponse{badRequest("Invalid value for parameter status: " + string(*request.Params.Status))}, nil
		}
		status = string(*request.Params.Status)
	}
	result, err := h.svc.ListClients(ctx, status)
	if err != nil {
		return ListNetworkClients500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListNetworkClients200JSONResponse(result), nil
}

// GetNetworkClient implements StrictServerInterface.
func (h *ServerHandler) GetNetworkClient(ctx context.Context, request GetNetworkClientRequestObject) (GetNetworkClientResponseObject, error) {
	detail, found, err := h.svc.GetClient(ctx, request.ClientId)
	if err != nil {
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetNetworkClient404ApplicationProblemPlusJSONResponse{notFound(err.Error())}, nil
		}
		return GetNetworkClient500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	if !found {
		return GetNetworkClient404ApplicationProblemPlusJSONResponse{notFound("Network client not found: " + request.ClientId)}, nil
	}
	return networkClientDetailResponse{detail: detail}, nil
}

// ListSsids implements StrictServerInterface.
func (h *ServerHandler) ListSsids(ctx context.Context, _ ListSsidsRequestObject) (ListSsidsResponseObject, error) {
	result, err := h.svc.ListSSIDs(ctx)
	if err != nil {
		return ListSsids500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListSsids200JSONResponse(result), nil
}

// GetSsid implements StrictServerInterface.
func (h *ServerHandler) GetSsid(ctx context.Context, request GetSsidRequestObject) (GetSsidResponseObject, error) {
	detail, found, err := h.svc.GetSSID(ctx, request.SsidId)
	if err != nil {
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetSsid404ApplicationProblemPlusJSONResponse{notFound(err.Error())}, nil
		}
		return GetSsid500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	if !found {
		return GetSsid404ApplicationProblemPlusJSONResponse{notFound("SSID not found: " + request.SsidId)}, nil
	}
	return GetSsid200JSONResponse(detail), nil
}

// ListVlans implements StrictServerInterface.
func (h *ServerHandler) ListVlans(ctx context.Context, _ ListVlansRequestObject) (ListVlansResponseObject, error) {
	result, err := h.svc.ListVLANs(ctx)
	if err != nil {
		return ListVlans500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListVlans200JSONResponse(result), nil
}

// GetVlan implements StrictServerInterface.
func (h *ServerHandler) GetVlan(ctx context.Context, request GetVlanRequestObject) (GetVlanResponseObject, error) {
	detail, found, err := h.svc.GetVLAN(ctx, request.VlanId)
	if err != nil {
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetVlan404ApplicationProblemPlusJSONResponse{notFound(err.Error())}, nil
		}
		return GetVlan500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	if !found {
		return GetVlan404ApplicationProblemPlusJSONResponse{notFound("VLAN not found: " + request.VlanId)}, nil
	}
	return GetVlan200JSONResponse(detail), nil
}

// ListWans implements StrictServerInterface.
func (h *ServerHandler) ListWans(ctx context.Context, _ ListWansRequestObject) (ListWansResponseObject, error) {
	result, err := h.svc.ListWANs(ctx)
	if err != nil {
		return ListWans500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListWans200JSONResponse(result), nil
}

// GetWan implements StrictServerInterface.
func (h *ServerHandler) GetWan(ctx context.Context, request GetWanRequestObject) (GetWanResponseObject, error) {
	detail, found, err := h.svc.GetWAN(ctx, request.WanId)
	if err != nil {
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetWan404ApplicationProblemPlusJSONResponse{notFound(err.Error())}, nil
		}
		return GetWan500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	if !found {
		return GetWan404ApplicationProblemPlusJSONResponse{notFound("WAN not found: " + request.WanId)}, nil
	}
	return GetWan200JSONResponse(detail), nil
}

// GetNetworkTopology implements StrictServerInterface.
func (h *ServerHandler) GetNetworkTopology(ctx context.Context, request GetNetworkTopologyRequestObject) (GetNetworkTopologyResponseObject, error) {
	includeClients := request.Params.IncludeClients != nil && *request.Params.IncludeClients
	result, err := h.svc.GetTopology(ctx, includeClients)
	if err != nil {
		return GetNetworkTopology500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return GetNetworkTopology200JSONResponse(result), nil
}
