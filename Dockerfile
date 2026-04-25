# Build the Go server binary in a full Go image, then copy into a
# minimal distroless runtime image.

FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /build/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /build/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
