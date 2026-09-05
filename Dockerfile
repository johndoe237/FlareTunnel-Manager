# syntax=docker/dockerfile:1
# Build FlareTunnel from the exact reviewed fork revision.
FROM golang:1.22-alpine AS flaretunnel-build
ARG FLARETUNNEL_REPO=https://github.com/johndoe237/FlareTunnel
ARG FLARETUNNEL_SHA=451990dcfc5abd1e698c4bf8aca312471799e07b
RUN apk add --no-cache git ca-certificates \
    && git clone "${FLARETUNNEL_REPO}" /src/flaretunnel \
    && cd /src/flaretunnel \
    && git checkout --detach "${FLARETUNNEL_SHA}" \
    && test "$(git rev-parse HEAD)" = "${FLARETUNNEL_SHA}" \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/flaretunnel .

# Build the manager as a complete Go package.
FROM golang:1.22-alpine AS manager-build
WORKDIR /src/manager
COPY go.mod ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/flaretunnel-manager ./cmd/manager

# One minimal runtime image for VPS, PaaS, and local Docker.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=flaretunnel-build /out/flaretunnel /usr/local/bin/flaretunnel
COPY --from=manager-build /out/flaretunnel-manager /usr/local/bin/flaretunnel-manager
COPY --from=flaretunnel-build /src/flaretunnel/blacklist-minimal.txt /opt/flaretunnel/blacklist-minimal.txt
COPY --from=flaretunnel-build /src/flaretunnel/blacklist.txt /opt/flaretunnel/blacklist.txt
COPY --from=flaretunnel-build /src/flaretunnel/blacklist-aggressive.txt /opt/flaretunnel/blacklist-aggressive.txt
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/flaretunnel-manager"]
