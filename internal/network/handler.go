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

// ListNetworkDevices implements StrictServerInterface.
func (h *ServerHandler) ListNetworkDevices(ctx context.Context, request ListNetworkDevicesRequestObject) (ListNetworkDevicesResponseObject, error) {
	result, err := h.svc.ListDevices(ctx)
	if err != nil {
		detail := err.Error()
		return ListNetworkDevices500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListNetworkDevices200JSONResponse(result), nil
}

// GetNetworkDevice implements StrictServerInterface.
func (h *ServerHandler) GetNetworkDevice(ctx context.Context, request GetNetworkDeviceRequestObject) (GetNetworkDeviceResponseObject, error) {
	detail, found, err := h.svc.GetDevice(ctx, request.DeviceId)
	if err != nil {
		msg := err.Error()
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetNetworkDevice404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNNotFound,
					Title:  apierrors.TitleNotFound,
					Status: 404,
					Detail: &msg,
				},
			}, nil
		}
		return GetNetworkDevice500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &msg,
			},
		}, nil
	}
	if !found {
		msg := "Network device not found: " + request.DeviceId
		return GetNetworkDevice404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
				Detail: &msg,
			},
		}, nil
	}
	return networkDeviceDetailResponse{detail: detail}, nil
}

// ListNetworkClients implements StrictServerInterface.
func (h *ServerHandler) ListNetworkClients(ctx context.Context, request ListNetworkClientsRequestObject) (ListNetworkClientsResponseObject, error) {
	var status string
	if request.Params.Status != nil {
		if !request.Params.Status.Valid() {
			detail := "Invalid value for parameter status: " + string(*request.Params.Status)
			return ListNetworkClients400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNBadRequest,
					Title:  apierrors.TitleBadRequest,
					Status: 400,
					Detail: &detail,
				},
			}, nil
		}
		status = string(*request.Params.Status)
	}
	result, err := h.svc.ListClients(ctx, status)
	if err != nil {
		detail := err.Error()
		return ListNetworkClients500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListNetworkClients200JSONResponse(result), nil
}

// GetNetworkClient implements StrictServerInterface.
func (h *ServerHandler) GetNetworkClient(ctx context.Context, request GetNetworkClientRequestObject) (GetNetworkClientResponseObject, error) {
	detail, found, err := h.svc.GetClient(ctx, request.ClientId)
	if err != nil {
		msg := err.Error()
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetNetworkClient404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNNotFound,
					Title:  apierrors.TitleNotFound,
					Status: 404,
					Detail: &msg,
				},
			}, nil
		}
		return GetNetworkClient500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &msg,
			},
		}, nil
	}
	if !found {
		msg := "Network client not found: " + request.ClientId
		return GetNetworkClient404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
				Detail: &msg,
			},
		}, nil
	}
	return networkClientDetailResponse{detail: detail}, nil
}

// ListSsids implements StrictServerInterface.
func (h *ServerHandler) ListSsids(ctx context.Context, _ ListSsidsRequestObject) (ListSsidsResponseObject, error) {
	result, err := h.svc.ListSSIDs(ctx)
	if err != nil {
		detail := err.Error()
		return ListSsids500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListSsids200JSONResponse(result), nil
}

// GetSsid implements StrictServerInterface.
func (h *ServerHandler) GetSsid(ctx context.Context, request GetSsidRequestObject) (GetSsidResponseObject, error) {
	detail, found, err := h.svc.GetSSID(ctx, request.SsidId)
	if err != nil {
		msg := err.Error()
		return GetSsid500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &msg,
			},
		}, nil
	}
	if !found {
		msg := "SSID not found: " + request.SsidId
		return GetSsid404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
				Detail: &msg,
			},
		}, nil
	}
	return GetSsid200JSONResponse(detail), nil
}

// ListVlans implements StrictServerInterface.
func (h *ServerHandler) ListVlans(ctx context.Context, _ ListVlansRequestObject) (ListVlansResponseObject, error) {
	result, err := h.svc.ListVLANs(ctx)
	if err != nil {
		detail := err.Error()
		return ListVlans500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListVlans200JSONResponse(result), nil
}

// GetVlan implements StrictServerInterface.
func (h *ServerHandler) GetVlan(ctx context.Context, request GetVlanRequestObject) (GetVlanResponseObject, error) {
	detail, found, err := h.svc.GetVLAN(ctx, request.VlanId)
	if err != nil {
		msg := err.Error()
		return GetVlan500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &msg,
			},
		}, nil
	}
	if !found {
		msg := "VLAN not found: " + request.VlanId
		return GetVlan404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
				Detail: &msg,
			},
		}, nil
	}
	return GetVlan200JSONResponse(detail), nil
}

// ListWans implements StrictServerInterface.
func (h *ServerHandler) ListWans(ctx context.Context, _ ListWansRequestObject) (ListWansResponseObject, error) {
	result, err := h.svc.ListWANs(ctx)
	if err != nil {
		detail := err.Error()
		return ListWans500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListWans200JSONResponse(result), nil
}

// GetWan implements StrictServerInterface.
func (h *ServerHandler) GetWan(ctx context.Context, request GetWanRequestObject) (GetWanResponseObject, error) {
	detail, found, err := h.svc.GetWAN(ctx, request.WanId)
	if err != nil {
		msg := err.Error()
		return GetWan500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &msg,
			},
		}, nil
	}
	if !found {
		msg := "WAN not found: " + request.WanId
		return GetWan404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNNotFound,
				Title:  apierrors.TitleNotFound,
				Status: 404,
				Detail: &msg,
			},
		}, nil
	}
	return GetWan200JSONResponse(detail), nil
}

// GetNetworkTopology implements StrictServerInterface.
func (h *ServerHandler) GetNetworkTopology(ctx context.Context, request GetNetworkTopologyRequestObject) (GetNetworkTopologyResponseObject, error) {
	includeClients := request.Params.IncludeClients != nil && *request.Params.IncludeClients
	result, err := h.svc.GetTopology(ctx, includeClients)
	if err != nil {
		detail := err.Error()
		return GetNetworkTopology500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return GetNetworkTopology200JSONResponse(result), nil
}
