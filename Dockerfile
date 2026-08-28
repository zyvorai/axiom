FROM golang:1.27 AS build
WORKDIR /src
COPY go.production.mod go.production.sum ./
RUN cp go.production.mod go.mod && cp go.production.sum go.sum && go mod download
COPY . .
RUN ./scripts/sync-ui.sh \
 && CGO_ENABLED=0 go build -modfile=go.production.mod -tags kubernetes \
      -trimpath -ldflags="-s -w" -o /out/zyvor-axiom ./cmd/zyvor-axiom

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/zyvor-axiom /zyvor-axiom
EXPOSE 5090
USER nonroot:nonroot
ENTRYPOINT ["/zyvor-axiom"]
