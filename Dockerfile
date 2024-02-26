FROM golang:1.21 AS builder

LABEL org.opencontainers.image.source="https://github.com/Cdayz/k8s-image-pre-puller"

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY . .

RUN go build -a -o pre-pull-image-controller cmd/pre-pull-image-controller/main.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/pre-pull-image-controller .
USER 65532:65532

ENTRYPOINT ["/pre-pull-image-controller"]