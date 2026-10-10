# syntax=docker/dockerfile:1.7

FROM node:22-bookworm-slim AS web
WORKDIR /src
COPY web/package.json web/package-lock.json ./web/
RUN --mount=type=cache,target=/root/.npm \
    cd web && npm ci
COPY web ./web
RUN mkdir -p pkg/server/dist && cd web && npm run build

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/pkg/server/dist ./pkg/server/dist

ARG VERSION=dev
ARG BRANCH=container
ARG COMMIT=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w -extldflags=-static \
      -X github.com/LosFurina/tmuxatlas/pkg/common.SUMMARY=${VERSION} \
      -X github.com/LosFurina/tmuxatlas/pkg/common.BRANCH=${BRANCH} \
      -X github.com/LosFurina/tmuxatlas/pkg/common.VERSION=${VERSION} \
      -X github.com/LosFurina/tmuxatlas/pkg/common.COMMIT=${COMMIT}" \
      -o /out/tmuxatlas .

RUN mkdir -p \
      /rootfs/etc/ssl/certs \
      /rootfs/var/lib/tmuxatlas/config \
      /rootfs/var/lib/tmuxatlas/data \
      /rootfs/var/lib/tmuxatlas/home \
      /rootfs/run/tmuxatlas \
      /rootfs/tmp \
    && cp /etc/ssl/certs/ca-certificates.crt /rootfs/etc/ssl/certs/ \
    && printf 'tmuxatlas:x:65532:65532:TmuxAtlas:/var/lib/tmuxatlas/home:/sbin/nologin\n' > /rootfs/etc/passwd \
    && printf 'tmuxatlas:x:65532:\n' > /rootfs/etc/group \
    && chown -R 65532:65532 /rootfs/var/lib/tmuxatlas /rootfs/run/tmuxatlas /rootfs/tmp

FROM scratch
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.title="TmuxAtlas" \
      org.opencontainers.image.description="Remote-only TmuxAtlas Hub" \
      org.opencontainers.image.source="https://github.com/LosFurina/tmuxatlas" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /rootfs/ /
COPY --from=build /out/tmuxatlas /usr/local/bin/tmuxatlas

ENV HOME=/var/lib/tmuxatlas/home \
    XDG_CONFIG_HOME=/var/lib/tmuxatlas/config \
    XDG_DATA_HOME=/var/lib/tmuxatlas/data \
    XDG_RUNTIME_DIR=/var/lib/tmuxatlas/run \
    TMUXATLAS_DEPLOYMENT=docker \
    TMUXATLAS_LISTEN=0.0.0.0:7654 \
    TMUXATLAS_SOCKET=/var/lib/tmuxatlas/run/tmuxatlas.sock

USER 65532:65532
VOLUME ["/var/lib/tmuxatlas"]
EXPOSE 7654
ENTRYPOINT ["/usr/local/bin/tmuxatlas"]
CMD ["hub"]
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=6 \
  CMD ["/usr/local/bin/tmuxatlas", "healthcheck", "--role", "hub", "--deployment", "docker"]
