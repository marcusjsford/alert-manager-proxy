FROM golang:1.26 AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/alertmanager-proxy ./cmd/alertmanager-proxy

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/alertmanager-proxy /alertmanager-proxy
EXPOSE 9095 9096
ENTRYPOINT ["/alertmanager-proxy"]
