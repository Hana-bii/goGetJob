FROM golang:1.25.0-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY configs ./configs

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/goGetJob ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
    && adduser -D -g '' appuser

WORKDIR /app

COPY --from=builder /out/goGetJob /app/goGetJob
COPY configs ./configs
COPY internal/prompts ./internal/prompts
COPY internal/skills ./internal/skills

ENV CONFIG_PATH=/app/configs/config.example.yaml

EXPOSE 8080

USER appuser

ENTRYPOINT ["/app/goGetJob"]
