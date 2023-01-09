FROM docker.io/library/golang:1.19-alpine as build-apex
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags="-extldflags=-static" \
    -o apex ./cmd/apex

FROM docker.io/library/golang:1.19-alpine as build-mkcert
ARG TARGETOS
ARG TARGETARCH

WORKDIR /
RUN apk add git && \
    git clone https://github.com/FiloSottile/mkcert && cd mkcert && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags "-extldflags=-static -X main.Version=$(git describe --tags)"

FROM fedora:36 as fedora

COPY --from=build-apex /src/apex /apex
COPY --from=build-mkcert /mkcert/mkcert /mkcert
RUN dnf update -y && \
    dnf install wireguard-tools -y && \
    dnf clean all -y && \
    rm -rf /var/cache/yum
