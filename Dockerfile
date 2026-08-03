# syntax=docker/dockerfile:1.7

FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/otherhost-ui ./cmd/otherhost-ui

FROM alpine:3.22

ARG OTHERHOST_UID=1000
ARG OTHERHOST_GID=1000

RUN apk add --no-cache ca-certificates openssh-client \
    && printf 'otherhost:x:%s:%s:Otherhost dashboard:/home/otherhost:/sbin/nologin\n' "$OTHERHOST_UID" "$OTHERHOST_GID" >> /etc/passwd \
    && mkdir -p /home/otherhost \
    && chown "$OTHERHOST_UID:$OTHERHOST_GID" /home/otherhost

COPY --from=builder /out/otherhost-ui /usr/local/bin/otherhost-ui

USER otherhost
EXPOSE 7842

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:7842/ || exit 1

ENTRYPOINT ["/usr/local/bin/otherhost-ui"]
