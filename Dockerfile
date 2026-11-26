FROM golang:1.25.7-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY apps/api ./apps/api
RUN CGO_ENABLED=0 go build -trimpath -o /out/half-km ./apps/api

FROM debian:bookworm-slim
COPY --from=build /out/half-km /usr/local/bin/half-km
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/half-km"]
