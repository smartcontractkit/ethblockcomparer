# Builder
FROM golang as builder

WORKDIR /go/src/github.com/smartcontractkit/ethblockcomparer
ADD . ./
RUN go get github.com/golang/dep/cmd/dep && dep ensure
RUN go install

# Final layer: ubuntu with binary
FROM ubuntu:18.04

ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update && apt-get install -y ca-certificates

WORKDIR /root

COPY --from=builder /go/bin/ethblockcomparer /usr/local/bin/

ENTRYPOINT ethblockcomparer
EXPOSE 8080
