package containers

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

func (h *ServerHandler) ListContainers(ctx context.Context, request ListContainersRequestObject) (ListContainersResponseObject, error) {
	result, err := h.svc.ListContainers(ctx, request.Params.Device)
	if err != nil {
		detail := err.Error()
		return ListContainers500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return ListContainers200JSONResponse(result), nil
}

func (h *ServerHandler) GetContainer(ctx context.Context, request GetContainerRequestObject) (GetContainerResponseObject, error) {
	result, err := h.svc.GetContainer(ctx, request.ContainerId)
	if err != nil {
		detail := err.Error()
		if errors.Is(err, apierrors.ErrNotFound) {
			return GetContainer404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNNotFound,
					Title:  apierrors.TitleNotFound,
					Status: 404,
					Detail: &detail,
				},
			}, nil
		}
		return GetContainer500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return GetContainer200JSONResponse(*result), nil
}

func (h *ServerHandler) RestartContainer(ctx context.Context, request RestartContainerRequestObject) (RestartContainerResponseObject, error) {
	if err := h.svc.RestartContainer(ctx, request.ContainerId); err != nil {
		detail := err.Error()
		if errors.Is(err, apierrors.ErrNotFound) {
			return RestartContainer404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNNotFound,
					Title:  apierrors.TitleNotFound,
					Status: 404,
					Detail: &detail,
				},
			}, nil
		}
		return RestartContainer500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return RestartContainer204Response{}, nil
}

func (h *ServerHandler) StartContainer(ctx context.Context, request StartContainerRequestObject) (StartContainerResponseObject, error) {
	if err := h.svc.StartContainer(ctx, request.ContainerId); err != nil {
		detail := err.Error()
		if errors.Is(err, apierrors.ErrNotFound) {
			return StartContainer404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNNotFound,
					Title:  apierrors.TitleNotFound,
					Status: 404,
					Detail: &detail,
				},
			}, nil
		}
		return StartContainer500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return StartContainer204Response{}, nil
}

func (h *ServerHandler) StopContainer(ctx context.Context, request StopContainerRequestObject) (StopContainerResponseObject, error) {
	if err := h.svc.StopContainer(ctx, request.ContainerId); err != nil {
		detail := err.Error()
		if errors.Is(err, apierrors.ErrNotFound) {
			return StopContainer404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse{
					Type:   apierrors.URNNotFound,
					Title:  apierrors.TitleNotFound,
					Status: 404,
					Detail: &detail,
				},
			}, nil
		}
		return StopContainer500ApplicationProblemPlusJSONResponse{
			InternalServerErrorApplicationProblemPlusJSONResponse{
				Type:   apierrors.URNInternalServerError,
				Title:  apierrors.TitleInternalServerError,
				Status: 500,
				Detail: &detail,
			},
		}, nil
	}
	return StopContainer204Response{}, nil
}
