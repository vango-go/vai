FROM golang:1.21 AS build

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/vai-proxy ./cmd/proxy

FROM gcr.io/distroless/static-debian12

WORKDIR /app
COPY --from=build /bin/vai-proxy /app/vai-proxy
COPY config/vai.yaml /etc/vai/config.yaml

ENV VAI_CONFIG=/etc/vai/config.yaml
EXPOSE 8080

ENTRYPOINT ["/app/vai-proxy"]
