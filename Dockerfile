# syntax=docker/dockerfile:1

# Multi-stage build: compile a static kilhog binary, then ship it in a
# scratch image (no shell, no package manager, minimal attack surface).

FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates \
	&& update-ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# modernc.org/sqlite is pure Go — no CGO required for a static binary.
RUN CGO_ENABLED=0 GOOS=linux go build \
	-trimpath \
	-ldflags="-s -w" \
	-o /out/kilhog \
	./cmd/kilhog

# Writable data directory for the non-root user in the final image.
RUN mkdir -p /out/data && chown 65532:65532 /out/data

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/kilhog /kilhog
COPY --from=builder --chown=65532:65532 /out/data /data

ENV KILHOG_HOST=0.0.0.0 \
	KILHOG_PORT=8080 \
	KILHOG_DB_DRIVER=sqlite \
	KILHOG_DB_DSN=file:/data/kilhog.db?_pragma=foreign_keys(ON)

EXPOSE 8080

USER 65532:65532

ENTRYPOINT ["/kilhog"]
