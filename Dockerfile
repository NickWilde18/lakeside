# syntax=docker/dockerfile:1.7

FROM golang:1.26-bookworm AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/lakeside .


FROM debian:bookworm-slim AS runtime

ENV TZ=Asia/Shanghai

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Config is expected to be mounted by k8s secret/configmap at runtime.
RUN mkdir -p /app/manifest/config /app/runtime

COPY --from=builder /out/lakeside /usr/local/bin/lakeside

EXPOSE 8011

ENTRYPOINT ["/usr/local/bin/lakeside"]
