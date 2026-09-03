# syntax=docker/dockerfile:1

# Multi-stage build: compile a static kilhog binary, then ship it in a
# scratch image (no shell, no package manager, minimal attack surface).

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates \
	&& update-ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# BuildKit sets TARGETOS/TARGETARCH for multi-platform images.
ARG TARGETOS=linux
ARG TARGETARCH=amd64

# modernc.org/sqlite is pure Go — no CGO required for a static binary.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
	-trimpath \
	-ldflags="-s -w" \
	-o /out/kilhog \
	./cmd/kilhog

# Writable data directory for the non-root user in the final image.
RUN mkdir -p /out/data /out/tmp && chown 65532:65532 /out/data /out/tmp

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder --chown=65532:65532 --chmod=755 /out/kilhog /kilhog
COPY --from=builder --chown=65532:65532 /out/data /data
COPY --from=builder --chown=65532:65532 /out/tmp /tmp

ENV KILHOG_HOST=0.0.0.0 \
	KILHOG_PORT=8080 \
	KILHOG_DB_DRIVER=sqlite \
	KILHOG_DB_DSN=file:/data/kilhog.db?_pragma=foreign_keys(ON)

EXPOSE 8080

USER 65532:65532

ENTRYPOINT ["/kilhog"]
