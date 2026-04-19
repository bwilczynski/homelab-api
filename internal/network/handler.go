package network

import (
	"context"
	"net/http"
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

// ServerHandler implements StrictServerInterface by delegating to the Service.
type ServerHandler struct {
	svc *Service
}

// NewHandler creates a new ServerHandler.
func NewHandler(svc *Service) *ServerHandler {
	return &ServerHandler{svc: svc}
}

// ListNetworkDevices implements StrictServerInterface.
func (h *ServerHandler) ListNetworkDevices(ctx context.Context, request ListNetworkDevicesRequestObject) (ListNetworkDevicesResponseObject, error) {
	result, err := h.svc.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	return ListNetworkDevices200JSONResponse(result), nil
}

// GetNetworkDevice implements StrictServerInterface.
func (h *ServerHandler) GetNetworkDevice(ctx context.Context, request GetNetworkDeviceRequestObject) (GetNetworkDeviceResponseObject, error) {
	detail, found, err := h.svc.GetDevice(ctx, request.DeviceId)
	if err != nil {
		return nil, err
	}
	if !found {
		msg := "Network device not found: " + request.DeviceId
		return GetNetworkDevice404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: NotFoundApplicationProblemPlusJSONResponse{
				Type:   "https://homelab.local/problems/not-found",
				Title:  "Not Found",
				Status: 404,
				Detail: &msg,
			},
		}, nil
	}
	return GetNetworkDevice200JSONResponse(detail), nil
}

// ListNetworkClients implements StrictServerInterface.
func (h *ServerHandler) ListNetworkClients(ctx context.Context, request ListNetworkClientsRequestObject) (ListNetworkClientsResponseObject, error) {
	result, err := h.svc.ListClients(ctx)
	if err != nil {
		return nil, err
	}
	return ListNetworkClients200JSONResponse(result), nil
}

// GetNetworkClient implements StrictServerInterface.
func (h *ServerHandler) GetNetworkClient(ctx context.Context, request GetNetworkClientRequestObject) (GetNetworkClientResponseObject, error) {
	detail, found, err := h.svc.GetClient(ctx, request.ClientId)
	if err != nil {
		return nil, err
	}
	if !found {
		msg := "Network client not found: " + request.ClientId
		return GetNetworkClient404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: NotFoundApplicationProblemPlusJSONResponse{
				Type:   "https://homelab.local/problems/not-found",
				Title:  "Not Found",
				Status: 404,
				Detail: &msg,
			},
		}, nil
	}
	return networkClientDetailResponse{detail: detail}, nil
}
