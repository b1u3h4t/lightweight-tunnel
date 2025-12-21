# UPnP Support Documentation

## Overview

Universal Plug and Play (UPnP) support has been added to improve P2P connectivity when clients are behind NAT routers that support UPnP/IGD (Internet Gateway Device).

## What is UPnP?

UPnP allows applications to automatically configure port forwarding on compatible routers, eliminating the need for manual port forwarding configuration. This significantly improves P2P connection success rates.

## Current Implementation Status

### ✅ Implemented Features

1. **SSDP Discovery**: Automatic discovery of UPnP-capable gateways on the local network
2. **Basic Framework**: Core UPnP client structure and error handling
3. **Logging**: Comprehensive logging for troubleshooting UPnP issues
4. **Best-effort Approach**: UPnP is attempted but failures don't prevent the application from working

### ⚠️ Limitations

The current implementation is a **basic framework** that:
- Discovers UPnP gateways using SSDP (Simple Service Discovery Protocol)
- Logs UPnP attempts and results
- Does NOT include full IGD (Internet Gateway Device) protocol implementation

**Why?** Full UPnP/IGD implementation requires complex SOAP (Simple Object Access Protocol) calls to interact with routers. This would significantly increase the codebase complexity.

### 🔧 Production Recommendation

For production deployments requiring full UPnP support, integrate a complete UPnP library such as:
- **github.com/huin/goupnp** - Comprehensive Go UPnP library
- **github.com/NebulousLabs/go-upnp** - Simplified UPnP interface

## How It Works

### Discovery Process

1. **Application Startup**: When P2P manager starts, it attempts UPnP discovery
2. **SSDP Multicast**: Sends discovery message to 239.255.255.250:1900
3. **Gateway Response**: UPnP-capable router responds with its location URL
4. **Port Mapping (Planned)**: Would configure port forwarding for P2P port

### Log Messages

```
UPnP: Starting discovery from local IP 192.168.1.100
UPnP: Discovered gateway at http://192.168.1.1:5000/rootDesc.xml
UPnP: Basic discovery completed. For full IGD port mapping, consider using a complete UPnP library
```

Or on failure:
```
UPnP: Discovery failed (continuing without UPnP): no UPnP gateway found
```

## Benefits

### With UPnP

- ✅ Automatic port forwarding on compatible routers
- ✅ No manual router configuration needed
- ✅ Higher P2P success rates
- ✅ Better for non-technical users

### Without UPnP

- ⚠️ Manual port forwarding required for optimal P2P
- ⚠️ Lower P2P success rates (but still works via STUN/hole-punching)
- ⚠️ More setup complexity for users

## Router Compatibility

UPnP support varies by router manufacturer and model:

| Router Type | UPnP Support | Notes |
|------------|--------------|-------|
| Home Routers (TP-Link, Netgear, etc.) | ✅ Usually Yes | May need to be enabled in settings |
| ISP-Provided Routers | ⚠️ Mixed | Often disabled for security |
| Enterprise Routers | ❌ Usually No | Disabled for security reasons |
| Mobile Hotspots | ❌ Usually No | Limited router functionality |

## Security Considerations

### UPnP Security Risks

1. **Automatic Port Opening**: Can be exploited by malicious software
2. **Network Exposure**: Opens ports without explicit user consent
3. **Router Vulnerabilities**: Some UPnP implementations have had security flaws

### Mitigation

- UPnP is **best-effort only** - failures don't break the application
- Port mappings are temporary (expire on router reboot)
- Users can disable UPnP on their routers if concerned
- P2P still works without UPnP via STUN/hole-punching

## Manual Port Forwarding

If UPnP fails or is disabled, users can manually configure port forwarding:

### Steps

1. **Find Router IP**: Usually 192.168.1.1 or 192.168.0.1
2. **Access Router Settings**: Open router IP in web browser
3. **Navigate to Port Forwarding**: Usually under "Advanced" or "NAT" settings
4. **Add Rule**:
   - Protocol: UDP
   - External Port: P2P port (default: auto-assigned, check logs)
   - Internal IP: Client's local IP
   - Internal Port: Same as external port
5. **Save and Apply**: Restart router if needed

### Example (TP-Link Router)

```
Advanced → NAT Forwarding → Virtual Servers
Service Name: Lightweight Tunnel P2P
External Port: 19000
Internal Port: 19000
Protocol: UDP
IP Address: 192.168.1.100
```

## Troubleshooting

### UPnP Discovery Fails

**Symptoms**: Log shows "UPnP: Discovery failed"

**Causes**:
- Router doesn't support UPnP
- UPnP is disabled on router
- Firewall blocking multicast traffic

**Solutions**:
1. Enable UPnP in router settings
2. Check firewall settings
3. Manual port forwarding as fallback
4. P2P still works via hole-punching

### UPnP Discovered But Port Mapping Fails

**Symptoms**: Gateway discovered but mapping not created

**Causes**:
- Current basic implementation doesn't include full IGD
- Router UPnP implementation is non-standard
- IGD service not available on router

**Solutions**:
1. For production: Integrate full UPnP library (github.com/huin/goupnp)
2. Manual port forwarding
3. P2P still works via hole-punching

## Future Enhancements

### Planned Improvements

1. **Full IGD Implementation**: Complete SOAP-based port mapping
2. **NAT-PMP Support**: Alternative to UPnP (used by Apple devices)
3. **Port Mapping Refresh**: Automatically renew temporary mappings
4. **Multiple Protocol Support**: Support both WANIPConnection and WANPPPConnection
5. **Mapping Verification**: Check if port mapping was successful
6. **Automatic Cleanup**: Remove mappings on application exit

### Integration Example (Future)

```go
import "github.com/huin/goupnp"

// Discover and map port
client, err := upnp.Discover()
if err == nil {
    err = client.Forward(externalPort, internalPort, "UDP", "Lightweight Tunnel", 0)
    if err == nil {
        log.Printf("UPnP: Successfully mapped port %d", externalPort)
    }
}
```

## References

- **UPnP Forum**: https://openconnectivity.org/developer/specifications/upnp-resources/upnp
- **IGD Specification**: http://upnp.org/specs/gw/UPnP-gw-InternetGatewayDevice-v2-Device.pdf
- **SSDP (Simple Service Discovery Protocol)**: Part of UPnP suite
- **NAT-PMP**: https://tools.ietf.org/html/rfc6886

## Conclusion

The current UPnP implementation provides a foundation for automatic port forwarding, but full functionality requires integration of a complete UPnP library. The system gracefully handles UPnP failures and maintains P2P functionality through STUN/hole-punching mechanisms.

For most users, the existing STUN-based P2P and manual port forwarding options are sufficient. UPnP is a nice-to-have feature that improves convenience when available.
