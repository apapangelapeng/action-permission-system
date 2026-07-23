FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /aps ./cmd/aps

FROM alpine:3.20
COPY --from=build /aps /usr/local/bin/aps
EXPOSE 8080
ENTRYPOINT ["aps"]
