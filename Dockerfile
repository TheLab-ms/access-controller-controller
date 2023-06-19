FROM golang:1.19 AS builder
WORKDIR /go/src/access-controller-controller
RUN CGO_ENABLED=0 go build .

FROM scratch
COPY --from=builder /go/src/access-controller-controller /
CMD ["/access-controller-controller"]
