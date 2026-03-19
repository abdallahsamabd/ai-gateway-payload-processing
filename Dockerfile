ARG BUILDER_IMAGE=golang:1.25
ARG BASE_IMAGE=gcr.io/distroless/static:nonroot

FROM ${BUILDER_IMAGE} AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s" \
    -o /bbr-server \
    ./cmd/main.go

FROM ${BASE_IMAGE}
COPY --from=builder /bbr-server /bbr-server
ENTRYPOINT ["/bbr-server"]
