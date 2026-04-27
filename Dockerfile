# Stage 1: Bundle the OpenAPI spec and generate server stubs.
FROM node:20-alpine AS spec
WORKDIR /build
COPY spec/openapi spec/openapi
COPY spec/redocly.yaml spec/
RUN npx --yes @redocly/cli@1.25.15 bundle spec/openapi/openapi.yaml -o spec/dist/openapi.bundled.yaml

FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=spec /build/spec/dist/openapi.bundled.yaml spec/dist/openapi.bundled.yaml
RUN go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest --config oapi-codegen-system.yaml spec/dist/openapi.bundled.yaml \
 && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest --config oapi-codegen-containers.yaml spec/dist/openapi.bundled.yaml \
 && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest --config oapi-codegen-storage.yaml spec/dist/openapi.bundled.yaml \
 && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest --config oapi-codegen-backups.yaml spec/dist/openapi.bundled.yaml \
 && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest --config oapi-codegen-network.yaml spec/dist/openapi.bundled.yaml
RUN CGO_ENABLED=0 go build -o /build/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /build/server /server
ENV LOG_FORMAT=json
EXPOSE 8080
ENTRYPOINT ["/server"]
