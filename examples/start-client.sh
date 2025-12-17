#!/bin/bash
# Example client startup script

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo "Please run as root (required for TUN device)"
    exit 1
fi

# Configuration - UPDATE SERVER_IP to your server's IP address
SERVER_IP="YOUR_SERVER_IP"
SERVER_PORT="9000"
TUNNEL_ADDR="10.0.0.2/24"
MTU=1400
FEC_DATA=10
FEC_PARITY=3

if [ "$SERVER_IP" = "YOUR_SERVER_IP" ]; then
    echo "ERROR: Please edit this script and set SERVER_IP to your server's IP address"
    exit 1
fi

echo "Starting lightweight tunnel client..."
echo "Server: $SERVER_IP:$SERVER_PORT"
echo "Tunnel Address: $TUNNEL_ADDR"
echo ""

./lightweight-tunnel \
    -m client \
    -r "$SERVER_IP:$SERVER_PORT" \
    -t "$TUNNEL_ADDR" \
    -mtu "$MTU" \
    -fec-data "$FEC_DATA" \
    -fec-parity "$FEC_PARITY"
