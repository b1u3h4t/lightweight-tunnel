#!/bin/bash
# Example server startup script

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo "Please run as root (required for TUN device)"
    exit 1
fi

# Configuration
LOCAL_ADDR="0.0.0.0:9000"
TUNNEL_ADDR="10.0.0.1/24"
MTU=1400
FEC_DATA=10
FEC_PARITY=3

echo "Starting lightweight tunnel server..."
echo "Local Address: $LOCAL_ADDR"
echo "Tunnel Address: $TUNNEL_ADDR"
echo ""

./lightweight-tunnel \
    -m server \
    -l "$LOCAL_ADDR" \
    -t "$TUNNEL_ADDR" \
    -mtu "$MTU" \
    -fec-data "$FEC_DATA" \
    -fec-parity "$FEC_PARITY"
