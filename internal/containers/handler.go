package containers

import "context"

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
		return nil, err
	}
	return ListContainers200JSONResponse(result), nil
}

func (h *ServerHandler) GetContainer(ctx context.Context, request GetContainerRequestObject) (GetContainerResponseObject, error) {
	result, err := h.svc.GetContainer(ctx, request.ContainerId)
	if err != nil {
		return nil, err
	}
	if result == nil {
		detail := "container not found"
		return GetContainer404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse{
				Type:   "urn:homelab:error:not-found",
				Title:  "Not Found",
				Status: 404,
				Detail: &detail,
			},
		}, nil
	}
	return GetContainer200JSONResponse(*result), nil
}

func (h *ServerHandler) RestartContainer(ctx context.Context, request RestartContainerRequestObject) (RestartContainerResponseObject, error) {
	if err := h.svc.RestartContainer(ctx, request.ContainerId); err != nil {
		return nil, err
	}
	return RestartContainer204Response{}, nil
}

func (h *ServerHandler) StartContainer(ctx context.Context, request StartContainerRequestObject) (StartContainerResponseObject, error) {
	if err := h.svc.StartContainer(ctx, request.ContainerId); err != nil {
		return nil, err
	}
	return StartContainer204Response{}, nil
}

func (h *ServerHandler) StopContainer(ctx context.Context, request StopContainerRequestObject) (StopContainerResponseObject, error) {
	if err := h.svc.StopContainer(ctx, request.ContainerId); err != nil {
		return nil, err
	}
	return StopContainer204Response{}, nil
}
