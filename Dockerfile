FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/karoo

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/karoo /usr/local/bin/karoo
COPY config.json /app/config.json
EXPOSE 3334 8080
ENTRYPOINT ["/usr/local/bin/karoo","-config","/app/config.json"]
