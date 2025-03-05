# Rollback Netplay Optimizations

This document explains the optimizations made to the Simple64 netplay server for improved rollback performance.

## Overview

The optimizations focus on several key areas:
1. UDP socket and packet handling
2. Buffer management for rollback mode
3. Input processing and distribution
4. Network traffic prioritization
5. Jitter detection and recovery

These changes aim to reduce jitter, improve stability, and provide a more consistent rollback netplay experience.

## Detailed Optimizations

### 1. UDP Socket Optimizations

- **Optimized Socket Buffer Size**: Set to 128KB, providing sufficient headroom for N64 netplay traffic without excessive memory usage. N64 games typically run at 30fps with small input packets.
- **Dynamic DSCP Traffic Marking**: Implemented a dual-tier system with AF31 (class 26) for regular packets and CS4 (class 32) for critical input packets, providing intelligent QoS prioritization.
- **Socket Timeout Handling**: Removed socket timeouts to ensure continuous packet processing even during periods of high network activity.
- **Concurrent Packet Processing**: Implemented a worker pool with 4 dedicated goroutines to process packets in parallel, reducing processing latency.

### 2. Buffer Management for Rollback Mode

- **Fixed Buffer Size**: Increased from 2 to 3 frames for enhanced stability, providing more room for network fluctuations without causing visible jitter.
- **Consistent Buffer Health**: Maintains a fixed buffer health value in rollback mode, preventing client-side adjustments that can cause inconsistent gameplay.
- **Zero Lag Reporting**: In rollback mode, lag is not reported to clients, allowing them to maintain proper prediction without unnecessary adjustments.
- **Conservative Lead Count Updates**: Lead count is only updated when significantly ahead (more than 1 frame) to prevent frequent buffer adjustments that can cause jitter.
- **Jitter Detection and Recovery**: Monitors for sudden changes in lag and implements a recovery period with conservative buffer management to stabilize gameplay.

### 3. Input Processing Improvements

- **Immediate Input Distribution**: When a client sends input, it's immediately distributed to all players to minimize delay.
- **Predictive Input Buffering**: Sends one extra frame of input data to improve client-side prediction accuracy.
- **Prioritized Packet Processing**: Critical game state packets are processed with higher priority than other traffic.
- **Non-blocking Channel-based Processing**: Uses a buffered channel system to manage packet processing without blocking the main UDP listener.

### 4. Client-Side Stability

- **Predictable Buffer Behavior**: By maintaining fixed buffer sizes and health values, clients can better predict game state without frequent adjustments.
- **Reduced Jitter**: The combination of fixed buffers, conservative updates, and jitter recovery helps reduce visible jitter during gameplay.
- **Improved Error Recovery**: Better handling of network fluctuations without impacting gameplay experience.
- **Packet Loss Metrics**: Real-time monitoring of packet loss percentage to help diagnose connection issues.

## Technical Implementation

The key files modified include:
- `internal/gameServer/server.go`: Buffer management, jitter detection, and rollback mode constants
- `internal/gameServer/udp.go`: UDP socket configuration, packet handling, and worker pool implementation
- `main.go`: Added direct rollback mode support via command-line flag

Key constants:
- `RollbackFixedBuffer`: Set to 3 frames (increased from 2)
- `RollbackBufferTarget`: Set to match buffer health expectations
- UDP socket buffer size: 128KB (appropriate for N64 netplay traffic)
- DSCP marking: Dynamic between AF31 (class 26) and CS4 (class 32)
- `JitterThreshold`: Set to 2 frames to detect jitter conditions
- `JitterRecoveryFrames`: Set to 10 frames for recovery period after jitter

## Usage

### Windows

To use the optimized server with rollback netcode:

```
./simple64-netplay-ROLLBACK.exe -rollback
```

Or with a custom port:

```
./simple64-netplay-ROLLBACK.exe -rollback -port 45001
```

### Linux/Ubuntu

First, make sure the executable has the proper permissions:

```bash
chmod +x simple64-netplay-ROLLBACK
```

Then run the server with the rollback flag:

```bash
./simple64-netplay-ROLLBACK -rollback
```

Or with a custom port:

```bash
./simple64-netplay-ROLLBACK -rollback -port 45001
```

For optimal performance on Linux, consider setting the following system parameters:

```bash
# Increase UDP buffer limits
sudo sysctl -w net.core.rmem_max=262144
sudo sysctl -w net.core.wmem_max=262144

# Reduce network latency
sudo sysctl -w net.ipv4.tcp_low_latency=1

# Prioritize gaming traffic
sudo sysctl -w net.core.netdev_max_backlog=5000
```

These settings can be made permanent by adding them to `/etc/sysctl.conf`.

## Troubleshooting

If you still experience jitter or instability:

1. **Network Issues**: Check your network connection for packet loss or high latency.
2. **Buffer Size**: Consider further increasing the fixed buffer size (requires code modification).
3. **Client Configuration**: Ensure clients are properly configured for rollback mode.
4. **System Resources**: Verify that your system has sufficient resources to run the emulator and server.
5. **Jitter Threshold**: If jitter detection seems too sensitive or not sensitive enough, adjust the `JitterThreshold` constant.
6. **Linux Permissions**: On Linux systems, ensure the server has appropriate permissions to set socket options and priorities.

## Performance Monitoring

The server now includes enhanced performance monitoring:
- Packet loss percentage calculation
- Detailed player status logging
- Jitter detection and recovery events
- Regular performance metric reporting

## Future Improvements

Potential areas for further optimization:
- Adaptive jitter threshold based on connection quality
- More sophisticated packet loss recovery
- Enhanced input prediction algorithms
- Performance profiling and optimization of critical paths
- Client-side prediction improvements 