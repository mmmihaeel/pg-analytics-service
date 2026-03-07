FROM golang:1.23-alpine AS dev
WORKDIR /app
RUN apk add --no-cache bash ca-certificates curl git make tzdata
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/pg-analytics-service ./cmd/api

FROM gcr.io/distroless/static-debian12 AS runtime
WORKDIR /
COPY --from=build /out/pg-analytics-service /pg-analytics-service
EXPOSE 3004
ENTRYPOINT ["/pg-analytics-service"]
