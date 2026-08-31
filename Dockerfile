# syntax=docker/dockerfile:1

FROM node:24.19.0-alpine AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend ./
ARG SPINOZA_VERSION=dev
ENV SPINOZA_VERSION=${SPINOZA_VERSION}
RUN npm run build

FROM golang:1.26.6-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG SPINOZA_VERSION=dev
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags "-s -w -X github.com/sophotechlabs/spinoza/internal/version.value=${SPINOZA_VERSION}" \
    -o /out/spinoza .

FROM alpine:3.22 AS tools
ARG TARGETARCH
ARG HELM_VERSION=4.2.2
ARG KUBECTL_VERSION=1.34.1
RUN apk add --no-cache ca-certificates curl tar
WORKDIR /tools
RUN curl -fsSLO "https://get.helm.sh/helm-v${HELM_VERSION}-linux-${TARGETARCH}.tar.gz" \
    && curl -fsSLO "https://get.helm.sh/helm-v${HELM_VERSION}-linux-${TARGETARCH}.tar.gz.sha256sum" \
    && sha256sum -c "helm-v${HELM_VERSION}-linux-${TARGETARCH}.tar.gz.sha256sum" \
    && tar -xzf "helm-v${HELM_VERSION}-linux-${TARGETARCH}.tar.gz" \
    && install -m 0755 "linux-${TARGETARCH}/helm" /tools/helm
RUN curl -fsSLo /tools/kubectl "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" \
    && curl -fsSLo /tools/kubectl.sha256 "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl.sha256" \
    && echo "$(cat /tools/kubectl.sha256)  /tools/kubectl" | sha256sum -c - \
    && chmod 0755 /tools/kubectl

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 65532 -h /var/lib/spinoza spinoza \
    && mkdir -p /var/lib/spinoza \
    && chown -R spinoza:spinoza /var/lib/spinoza
COPY --from=build /out/spinoza /usr/local/bin/spinoza
COPY --from=tools /tools/helm /usr/local/bin/helm
COPY --from=tools /tools/kubectl /usr/local/bin/kubectl
USER 65532:65532
ENV HOME=/var/lib/spinoza \
    XDG_CONFIG_HOME=/var/lib/spinoza \
    XDG_CACHE_HOME=/var/lib/spinoza/cache \
    HELM_CACHE_HOME=/var/lib/spinoza/cache/helm \
    HELM_CONFIG_HOME=/var/lib/spinoza/helm \
    HELM_DATA_HOME=/var/lib/spinoza/helm/data \
    SPINOZA_CLUSTER_MODE=true \
    SPINOZA_ADDR=0.0.0.0:8080
WORKDIR /var/lib/spinoza
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
    CMD wget -q -O- http://127.0.0.1:8080/healthz > /dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/spinoza"]
