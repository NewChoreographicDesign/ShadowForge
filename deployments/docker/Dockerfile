# ShadowForge L1 node image (spec 18.1 deployments/docker/).
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/node ./cmd/node && \
    CGO_ENABLED=0 go build -o /out/walletsim ./cmd/walletsim && \
    CGO_ENABLED=0 go build -o /out/shadowc ./cmd/shadowc && \
    CGO_ENABLED=0 go build -o /out/explorer ./cmd/explorer

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/node /usr/local/bin/node
COPY --from=build /out/walletsim /usr/local/bin/walletsim
COPY --from=build /out/shadowc /usr/local/bin/shadowc
COPY --from=build /out/explorer /usr/local/bin/explorer
ENTRYPOINT ["/usr/local/bin/node"]
