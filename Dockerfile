# Stage 1: Bundle the OpenAPI spec and generate server stubs.
FROM node:20-alpine AS spec
WORKDIR /build
COPY spec/openapi spec/openapi
COPY spec/redocly.yaml spec/
RUN npx --yes @redocly/cli@1.25.15 bundle spec/openapi/openapi.yaml -o spec/dist/openapi.bundled.yaml

FROM golang:1.26-alpine AS builder
WORKDIR /build
RUN apk add --no-cache make
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=spec /build/spec/dist/openapi.bundled.yaml spec/dist/openapi.bundled.yaml
RUN SKIP_BUNDLE=true make generate
RUN make build

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /build/bin/server /server
ENV LOG_FORMAT=json
EXPOSE 8080
ENTRYPOINT ["/server"]
