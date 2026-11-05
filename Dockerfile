# Build on the host's platform and cross-compile, so multi-arch builds need no emulation.
FROM --platform=$BUILDPLATFORM golang:1.23 AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-X github.com/edward-murrell/k5pwdserver/internal/version.Version=${VERSION}" \
    -o /out/k5pwdserver ./cmd/pwdserver

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/k5pwdserver /usr/sbin/k5pwdserver
ENV GIN_MODE=release
EXPOSE 8080
# Mount /etc/krb5.conf, plus any TLS certificate and key, from the host.
ENTRYPOINT ["/usr/sbin/k5pwdserver"]
CMD ["-listen", ":8080"]
