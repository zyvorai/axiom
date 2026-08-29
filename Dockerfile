FROM node:20-slim AS ui
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm install --legacy-peer-deps
COPY frontend/ ./
RUN npx craco build

FROM golang:1.27 AS build
ARG AXIOM_VERSION=dev
WORKDIR /src
COPY go.production.mod go.production.sum ./
RUN cp go.production.mod go.mod && cp go.production.sum go.sum && go mod download
COPY . .
COPY --from=ui /src/frontend/build/ ./internal/ui/dist/
RUN CGO_ENABLED=0 go build -modfile=go.production.mod -tags kubernetes \
      -trimpath -ldflags="-s -w -X github.com/zyvorai/axiom/internal/api.buildVersion=${AXIOM_VERSION}" \
      -o /out/zyvor-axiom ./cmd/zyvor-axiom

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/zyvor-axiom /zyvor-axiom
EXPOSE 5090
USER nonroot:nonroot
ENTRYPOINT ["/zyvor-axiom"]
