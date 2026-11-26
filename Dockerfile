FROM node:24-bookworm AS web
WORKDIR /web
COPY apps/web/package.json apps/web/package-lock.json ./
RUN npm ci --registry https://registry.npmjs.org
COPY apps/web/ ./
RUN npm run build

FROM golang:1.25.7-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY apps/api ./apps/api
RUN CGO_ENABLED=0 go build -trimpath -o /out/half-km ./apps/api

FROM debian:bookworm-slim
COPY --from=build /out/half-km /usr/local/bin/half-km
COPY --from=web /web/dist /usr/local/share/half-km/web
ENV HALFKM_WEB_DIR=/usr/local/share/half-km/web
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/half-km"]
