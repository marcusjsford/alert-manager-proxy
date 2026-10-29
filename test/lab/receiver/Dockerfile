FROM golang:1.26 AS build
WORKDIR /src
COPY main.go ./main.go
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/receiver main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/receiver /receiver
EXPOSE 8080
ENTRYPOINT ["/receiver"]
