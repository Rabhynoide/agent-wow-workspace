FROM golang:1.27.1 AS build
ARG CMD
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /module ./cmd/${CMD}

FROM scratch
COPY --from=build /module /module
ENTRYPOINT ["/module"]
