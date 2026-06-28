package meta

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

// GetMetaVersion implements StrictServerInterface.
func (h *ServerHandler) GetMetaVersion(ctx context.Context, _ GetMetaVersionRequestObject) (GetMetaVersionResponseObject, error) {
	apiVersion, serverVersion := h.svc.GetVersion()
	return GetMetaVersion200JSONResponse{
		ApiVersion:    apiVersion,
		ServerVersion: serverVersion,
	}, nil
}

// GetMetaAuth implements StrictServerInterface.
func (h *ServerHandler) GetMetaAuth(ctx context.Context, _ GetMetaAuthRequestObject) (GetMetaAuthResponseObject, error) {
	enabled, issuer := h.svc.GetAuth()
	resp := GetMetaAuth200JSONResponse{Enabled: enabled}
	if issuer != "" {
		resp.Issuer = &issuer
	}
	return resp, nil
}
