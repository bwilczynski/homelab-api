package network

import "context"

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

// ListNetworkClients implements StrictServerInterface.
func (h *ServerHandler) ListNetworkClients(ctx context.Context, request ListNetworkClientsRequestObject) (ListNetworkClientsResponseObject, error) {
	result, err := h.svc.ListClients(ctx)
	if err != nil {
		return nil, err
	}
	return ListNetworkClients200JSONResponse(result), nil
}
