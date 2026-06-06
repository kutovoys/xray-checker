FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.25-alpine AS builder

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG GIT_TAG=unknown
ARG GIT_COMMIT=unknown
ARG USERNAME=kutovoys
ARG REPOSITORY_NAME=xray-checker

ENV CGO_ENABLED=0
ENV GO111MODULE=on

# Install UPX for binary compression
RUN apk add --no-cache upx

WORKDIR /go/src/github.com/${USERNAME}/${REPOSITORY_NAME}

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY . .

RUN CGO_ENABLED=${CGO_ENABLED} GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
  go build -ldflags="-s -w -X main.version=${GIT_TAG} -X main.commit=${GIT_COMMIT}" -a -installsuffix cgo -o /usr/bin/xray-checker . && \
  upx --best --lzma /usr/bin/xray-checker

FROM alpine:3.21

ARG USERNAME=kutovoys
ARG REPOSITORY_NAME=xray-checker

LABEL org.opencontainers.image.source=https://github.com/${USERNAME}/${REPOSITORY_NAME}

RUN apk add --no-cache ca-certificates curl tzdata && \
    adduser -D -u 1000 appuser && \
    mkdir -p /app/geo && \
    chown -R appuser:appuser /app

WORKDIR /app
COPY --from=builder /usr/bin/xray-checker /usr/bin/xray-checker

USER appuser

ENTRYPOINT ["/usr/bin/xray-checker"]
