package containers

import "context"

type ServerHandler struct {
	svc *Service
}

func NewHandler(svc *Service) *ServerHandler {
	return &ServerHandler{svc: svc}
}

func (h *ServerHandler) ListContainers(ctx context.Context, request ListContainersRequestObject) (ListContainersResponseObject, error) {
	return nil, nil
}

func (h *ServerHandler) GetContainer(ctx context.Context, request GetContainerRequestObject) (GetContainerResponseObject, error) {
	return nil, nil
}

func (h *ServerHandler) RestartContainer(ctx context.Context, request RestartContainerRequestObject) (RestartContainerResponseObject, error) {
	return nil, nil
}

func (h *ServerHandler) StartContainer(ctx context.Context, request StartContainerRequestObject) (StartContainerResponseObject, error) {
	return nil, nil
}

func (h *ServerHandler) StopContainer(ctx context.Context, request StopContainerRequestObject) (StopContainerResponseObject, error) {
	return nil, nil
}
