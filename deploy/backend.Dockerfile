# Production image of the API: a static Go binary on a distroless, non-root base.
# Build context: ../backend (see docker-compose.prod.yml).

FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/api ./cmd/api \
 && go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck \
 && go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/seed ./cmd/seed

FROM gcr.io/distroless/static-debian12:nonroot
# /seed creates the admin user once: docker compose exec api /seed
COPY --from=build /out/api /out/healthcheck /out/seed /
USER nonroot:nonroot
ENV HTTP_ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=3 CMD ["/healthcheck"]
ENTRYPOINT ["/api"]
