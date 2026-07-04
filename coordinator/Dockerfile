# Barycenter (duet coordinator) — production image for Coolify (V2-G1).
# Build context: repository root (needs protocol/golden for tests).

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY coordinator/go.mod coordinator/go.sum ./coordinator/
RUN cd coordinator && go mod download
COPY protocol/ ./protocol/
COPY coordinator/ ./coordinator/
ARG VERSION=dev
RUN cd coordinator && CGO_ENABLED=0 go build \
    -ldflags "-X main.version=${VERSION}" \
    -o /out/duet-coordinator ./cmd/duet-coordinator

FROM alpine:3.20
# ffmpeg + ffprobe: voice pipeline (spec ch. 10)
RUN apk add --no-cache ffmpeg ca-certificates tzdata \
    && addgroup -S duet && adduser -S duet -G duet \
    && mkdir -p /var/lib/duet/media /etc/duet \
    && chown -R duet:duet /var/lib/duet /etc/duet
COPY --from=build /out/duet-coordinator /usr/local/bin/duet-coordinator
COPY deploy/coordinator.container.yml /etc/duet/coordinator.yml
USER duet
VOLUME /var/lib/duet
EXPOSE 8080
# Secrets and per-install values come from env (see deploy/coolify.env.example).
ENTRYPOINT ["/usr/local/bin/duet-coordinator", "--config", "/etc/duet/coordinator.yml"]
