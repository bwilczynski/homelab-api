package docker

import (
	"context"
	"errors"

	"github.com/bwilczynski/homelab-api/internal/apierrors"
)

// ServerHandler implements the generated StrictServerInterface for containers.
type ServerHandler struct {
	svc *Service
}

// NewHandler creates a new container handler.
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

func (h *ServerHandler) ListContainers(ctx context.Context, request ListContainersRequestObject) (ListContainersResponseObject, error) {
	result, err := h.svc.ListContainers(ctx, request.Params.Device)
	if err != nil {
		return ListContainers500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListContainers200JSONResponse(result), nil
}

func (h *ServerHandler) GetContainer(ctx context.Context, request GetContainerRequestObject) (GetContainerResponseObject, error) {
	result, err := h.svc.GetContainer(ctx, request.ContainerId)
	if err != nil {
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetContainer404ApplicationProblemPlusJSONResponse{notFound(err.Error())}, nil
		}
		return GetContainer500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return GetContainer200JSONResponse(*result), nil
}

func (h *ServerHandler) RestartContainer(ctx context.Context, request RestartContainerRequestObject) (RestartContainerResponseObject, error) {
	if err := h.svc.RestartContainer(ctx, request.ContainerId); err != nil {
		if errors.Is(err, apierrors.ErrNotFound) {
			return RestartContainer404ApplicationProblemPlusJSONResponse{notFound(err.Error())}, nil
		}
		return RestartContainer500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return RestartContainer204Response{}, nil
}

func (h *ServerHandler) StartContainer(ctx context.Context, request StartContainerRequestObject) (StartContainerResponseObject, error) {
	if err := h.svc.StartContainer(ctx, request.ContainerId); err != nil {
		if errors.Is(err, apierrors.ErrNotFound) {
			return StartContainer404ApplicationProblemPlusJSONResponse{notFound(err.Error())}, nil
		}
		return StartContainer500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return StartContainer204Response{}, nil
}

func (h *ServerHandler) StopContainer(ctx context.Context, request StopContainerRequestObject) (StopContainerResponseObject, error) {
	if err := h.svc.StopContainer(ctx, request.ContainerId); err != nil {
		if errors.Is(err, apierrors.ErrNotFound) {
			return StopContainer404ApplicationProblemPlusJSONResponse{notFound(err.Error())}, nil
		}
		return StopContainer500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return StopContainer204Response{}, nil
}

func (h *ServerHandler) ListDockerImages(ctx context.Context, request ListDockerImagesRequestObject) (ListDockerImagesResponseObject, error) {
	result, err := h.svc.ListImages(ctx, request.Params.Device)
	if err != nil {
		return ListDockerImages500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListDockerImages200JSONResponse(result), nil
}

func (h *ServerHandler) GetDockerImage(ctx context.Context, request GetDockerImageRequestObject) (GetDockerImageResponseObject, error) {
	result, err := h.svc.GetImage(ctx, request.ImageId)
	if err != nil {
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetDockerImage404ApplicationProblemPlusJSONResponse{notFound(err.Error())}, nil
		}
		return GetDockerImage500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return GetDockerImage200JSONResponse(*result), nil
}

func (h *ServerHandler) ListDockerNetworks(ctx context.Context, request ListDockerNetworksRequestObject) (ListDockerNetworksResponseObject, error) {
	result, err := h.svc.ListNetworks(ctx, request.Params.Device)
	if err != nil {
		return ListDockerNetworks500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return ListDockerNetworks200JSONResponse(result), nil
}

func (h *ServerHandler) GetDockerNetwork(ctx context.Context, request GetDockerNetworkRequestObject) (GetDockerNetworkResponseObject, error) {
	result, err := h.svc.GetNetwork(ctx, request.NetworkId)
	if err != nil {
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetDockerNetwork404ApplicationProblemPlusJSONResponse{notFound(err.Error())}, nil
		}
		return GetDockerNetwork500ApplicationProblemPlusJSONResponse{internalServerError(err.Error())}, nil
	}
	return GetDockerNetwork200JSONResponse(*result), nil
}
