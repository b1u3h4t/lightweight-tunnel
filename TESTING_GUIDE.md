# Testing Guide for P2P and Tunnel Connectivity Fixes

This guide helps you verify the fixes for P2P routing and server tunnel connectivity issues.

## Test Scenario 1: Server Tunnel IP Accessibility

**Problem Fixed**: Clients couldn't ping or access the server's tunnel IP address.

### Setup

**Server** (with public IP or port forwarding):
```bash
sudo ./lightweight-tunnel \
  -m server \
  -l 0.0.0.0:9000 \
  -t 10.0.0.1/24 \
  -k "test-key-2024"
```

**Client**:
```bash
sudo ./lightweight-tunnel \
  -m client \
  -r <SERVER_IP>:9000 \
  -t 10.0.0.2/24 \
  -k "test-key-2024"
```

### Test Steps

1. Wait for client to connect (you should see "Connected to server" in client logs)

2. **Test ICMP ping to server**:
   ```bash
   # On client machine
   ping 10.0.0.1
   ```
   
   **Expected Result**: ✅ Ping should succeed with responses from 10.0.0.1
   ```
   PING 10.0.0.1 (10.0.0.1) 56(84) bytes of data.
   64 bytes from 10.0.0.1: icmp_seq=1 ttl=64 time=1.23 ms
   64 bytes from 10.0.0.1: icmp_seq=2 ttl=64 time=0.98 ms
   ```

3. **Test TCP connection to server**:
   ```bash
   # On server, start a simple HTTP server on tunnel IP
   python3 -c "import http.server; http.server.test(bind='10.0.0.1')" &
   
   # On client
   curl http://10.0.0.1:8000/
   ```
   
   **Expected Result**: ✅ HTTP request should succeed

---

## Test Scenario 2: P2P Routing Fallback

**Problem Fixed**: When P2P fails, routing table wasn't properly switching to server relay.

### Setup

Same as Test Scenario 1, but with P2P enabled:

**Server**:
```bash
sudo ./lightweight-tunnel \
  -m server \
  -l 0.0.0.0:9000 \
  -t 10.0.0.1/24 \
  -k "test-key-2024"
```

**Client A**:
```bash
sudo ./lightweight-tunnel \
  -m client \
  -r <SERVER_IP>:9000 \
  -t 10.0.0.10/24 \
  -k "test-key-2024" \
  -p2p \
  -p2p-port 19000
```

**Client B**:
```bash
sudo ./lightweight-tunnel \
  -m client \
  -r <SERVER_IP>:9000 \
  -t 10.0.0.20/24 \
  -k "test-key-2024" \
  -p2p \
  -p2p-port 19001
```

### Test Steps

1. Wait for both clients to connect

2. **Check routing logs** (appears every 30 seconds by default):
   ```
   Routing stats: 2 peers, X direct, 0 relay, Y server
     Peer 10.0.0.20: route=P2P-DIRECT quality=120 status=connected throughServer=false
   ```
   or if P2P fails:
   ```
   Routing stats: 2 peers, 0 direct, 0 relay, 2 server
     Peer 10.0.0.20: route=SERVER-RELAY quality=70 status=disconnected throughServer=true
   ```

3. **Test connectivity between clients**:
   ```bash
   # On Client A
   ping 10.0.0.20
   ```
   
   **Expected Result**: ✅ Ping should work regardless of whether P2P succeeds
   - If P2P works: Low latency, direct connection
   - If P2P fails: Higher latency through server relay, but still works

4. **Verify routing decisions in logs**:
   - Look for log entries showing route type and throughServer flag
   - Verify that when "P2P send failed" appears, the next routing stats show route=SERVER-RELAY

---

## Test Scenario 3: P2P Success Detection

**Problem Fixed**: Routing table now properly reflects when P2P is actually working.

### Setup

Same as Test Scenario 2.

### Test Steps

1. Wait for P2P handshake to complete (should see "P2P connection established")

2. **Check routing logs**:
   ```
   Routing stats: 1 peers, 1 direct, 0 relay, 0 server
     Peer 10.0.0.20: route=P2P-DIRECT quality=120 status=connected throughServer=false
   ```
   
   **Expected Result**: 
   - ✅ `route=P2P-DIRECT` indicates P2P is being used
   - ✅ `throughServer=false` confirms traffic is NOT going through server
   - ✅ High quality score (100+) for P2P connections

3. **Block P2P traffic** (simulate P2P failure):
   ```bash
   # On Client A, block UDP traffic to Client B's P2P port
   sudo iptables -A OUTPUT -p udp --dport 19001 -j DROP
   ```

4. **Verify automatic fallback**:
   - Wait 30 seconds for routing update
   - Check logs should show:
   ```
   P2P send failed to 10.0.0.20, falling back to server: ...
   Routing stats: 1 peers, 0 direct, 0 relay, 1 server
     Peer 10.0.0.20: route=SERVER-RELAY quality=70 status=connected throughServer=true
   ```
   
   **Expected Result**: 
   - ✅ `route=SERVER-RELAY` indicates server relay is now being used
   - ✅ `throughServer=true` confirms routing state is correct
   - ✅ Ping still works but with higher latency

5. **Unblock and verify recovery**:
   ```bash
   sudo iptables -D OUTPUT -p udp --dport 19001 -j DROP
   ```
   
   After next routing update:
   ```
   Routing stats: 1 peers, 1 direct, 0 relay, 0 server
     Peer 10.0.0.20: route=P2P-DIRECT quality=120 status=connected throughServer=false
   ```

---

## Expected Log Output Examples

### Successful P2P Connection
```
P2P: Trying local address 192.168.1.100:19001 for peer 10.0.0.20
P2P: Local connection SUCCEEDED to 10.0.0.20 via 192.168.1.100:19001
P2P LOCAL connection established with 10.0.0.20 via 192.168.1.100:19001
Routing stats: 1 peers, 1 direct, 0 relay, 0 server
  Peer 10.0.0.20: route=P2P-DIRECT quality=150 status=connected-local throughServer=false
```

### P2P Failure with Server Fallback
```
P2P: Local connection to 10.0.0.20 failed, falling back to public address
P2P: Failed to resolve public address
P2P send failed to 10.0.0.20, falling back to server: no P2P connection to 10.0.0.20
Routing stats: 1 peers, 0 direct, 0 relay, 1 server
  Peer 10.0.0.20: route=SERVER-RELAY quality=70 status=disconnected throughServer=true
```

### Server Responding to Ping
```
# Server logs should show TUN read activity when client pings server IP
TUN read: 64 bytes (ICMP echo request from 10.0.0.2 to 10.0.0.1)
TUN write: 64 bytes (ICMP echo reply from 10.0.0.1 to 10.0.0.2)
```

---

## Troubleshooting

### If ping to server fails:

1. Check server TUN device:
   ```bash
   ip addr show tun0  # Should show 10.0.0.1
   ```

2. Check server logs for "TUN read error" or "TUN write error"

3. Verify firewall rules:
   ```bash
   sudo iptables -L -n | grep tun0
   ```

### If P2P never establishes:

1. Check NAT type (both clients should not be strict symmetric NAT)

2. Verify UDP port accessibility:
   ```bash
   sudo netstat -ulnp | grep <p2p-port>
   ```

3. Check for firewall blocking UDP:
   ```bash
   sudo ufw status
   sudo firewall-cmd --list-all
   ```

### If routing stats show incorrect state:

1. Wait for routing update interval (default 30 seconds)

2. Check for "P2P send failed" messages in logs

3. Verify P2P handshake completed:
   ```
   P2P connection established with 10.0.0.X
   ```

---

## Success Criteria

All tests pass when:

1. ✅ Client can ping server's tunnel IP (10.0.0.1)
2. ✅ Routing logs accurately show P2P-DIRECT when P2P works
3. ✅ Routing logs show SERVER-RELAY when P2P fails  
4. ✅ `throughServer` flag matches actual routing path
5. ✅ Quality scores are correct (high for P2P, lower for server relay)
6. ✅ Automatic fallback from P2P to server works seamlessly
7. ✅ Connectivity is maintained even when P2P fails
