# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /quiver-rss .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /quiver-rss /quiver-rss
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/quiver-rss"]
