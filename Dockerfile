FROM golang:1.24-bookworm AS builder

ENV GOPROXY=https://goproxy.cn,direct

RUN apt-get update && apt-get install -y --no-install-recommends libpcap-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags "-s -w" -o /lightweight-tunnel ./cmd/lightweight-tunnel

# -------------------------------------------------------------------
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        iproute2 iptables iputils-ping libpcap0.8 procps net-tools tcpdump netcat-openbsd && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /lightweight-tunnel /usr/local/bin/lightweight-tunnel

ENTRYPOINT ["lightweight-tunnel"]
