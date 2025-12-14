# Service Routing Configuration

This document describes the service routing configuration feature in Banyan, which allows you to control how incoming proxied requests are routed to your services.

## Overview

When a node receives a proxied request through libp2p, it uses the service key to look up routing configuration and forward the request to the appropriate backend service. The routing configuration allows you to:

1. **Specify the backend URL** where the service is running
2. **Add path prefixes** to incoming requests
3. **Control path handling** (keep full path or strip it)

## Configuration Format

Service routing is configured in the `services.json` file. Each service entry can include the following routing fields:

```json
{
  "service-name": {
    "directory": "service-directory",
    "pem": "service.pem",
    "fig": "service.fig.json",
    "routeUrl": "http://localhost:8080",
    "routePrefix": "/api/v1",
    "keepFullPath": true
  }
}
```

### Configuration Fields

#### Required Fields (existing)
- **`directory`**: Directory containing service files (relative to services directory)
- **`pem`** or **`pems`**: Service key file(s)
- **`fig`** or **`figs`**: Fig configuration file(s)

#### Routing Fields (new)
- **`routeUrl`**: Backend URL where the service is running (e.g., `http://localhost:8080`)
- **`routePrefix`**: Path prefix to add to incoming requests (optional)
- **`keepFullPath`**: Boolean flag controlling path handling (default: false)

## Routing Behavior

The routing behavior depends on the combination of `routePrefix` and `keepFullPath`:

### Case 1: `keepFullPath: true` with `routePrefix`

**Configuration:**
```json
{
  "routeUrl": "http://localhost:8080",
  "routePrefix": "/api/v1",
  "keepFullPath": true
}
```

**Behavior:** Prepends the prefix to the incoming path

**Example:**
- Incoming: `/router/{serviceKey}/users/123`
- Forwarded to: `http://localhost:8080/api/v1/users/123`

### Case 2: `keepFullPath: true` without `routePrefix`

**Configuration:**
```json
{
  "routeUrl": "http://localhost:8080",
  "keepFullPath": true
}
```

**Behavior:** Forwards the path as-is

**Example:**
- Incoming: `/router/{serviceKey}/users/123`
- Forwarded to: `http://localhost:8080/users/123`

### Case 3: `keepFullPath: false` with `routePrefix`

**Configuration:**
```json
{
  "routeUrl": "http://localhost:8080",
  "routePrefix": "/api/v1",
  "keepFullPath": false
}
```

**Behavior:** Strips the incoming path and uses only the prefix

**Example:**
- Incoming: `/router/{serviceKey}/users/123`
- Forwarded to: `http://localhost:8080/api/v1`

### Case 4: `keepFullPath: false` without `routePrefix`

**Configuration:**
```json
{
  "routeUrl": "http://localhost:8080",
  "keepFullPath": false
}
```

**Behavior:** Strips the incoming path, forwards to root

**Example:**
- Incoming: `/router/{serviceKey}/users/123`
- Forwarded to: `http://localhost:8080/`

### Case 5: No routing configuration

**Behavior:** Forwards the path as-is (same as Case 2)

**Example:**
- Incoming: `/router/{serviceKey}/users/123`
- Forwarded to: `http://localhost:8080/users/123`

## Complete Examples

### Example 1: REST API with versioned prefix

Your API runs on `http://localhost:8080` and expects paths like `/api/v1/users`, `/api/v1/posts`, etc.

**Configuration:**
```json
{
  "my-api": {
    "directory": "my-api",
    "pem": "service.pem",
    "fig": "service.fig.json",
    "routeUrl": "http://localhost:8080",
    "routePrefix": "/api/v1",
    "keepFullPath": true
  }
}
```

**Request flow:**
```
Client -> /proxy/alias/myapp/users
  -> Hierarchical match finds service key: 08011220abc...
  -> Peer: libp2p://12D3Koo.../router/08011220abc.../users
  -> Backend: http://localhost:8080/api/v1/users
```

### Example 2: Simple web service

Your web service runs on `http://localhost:3000` and expects paths like `/`, `/about`, `/contact`, etc.

**Configuration:**
```json
{
  "web-service": {
    "directory": "web-service",
    "pem": "service.pem",
    "fig": "service.fig.json",
    "routeUrl": "http://localhost:3000",
    "keepFullPath": true
  }
}
```

**Request flow:**
```
Client -> /proxy/alias/myapp/about
  -> Peer: libp2p://12D3Koo.../router/08011220xyz.../about
  -> Backend: http://localhost:3000/about
```

### Example 3: Database proxy with fixed endpoint

Your database proxy runs on `http://localhost:5432` and all requests should go to `/db` regardless of the incoming path.

**Configuration:**
```json
{
  "database-proxy": {
    "directory": "database-proxy",
    "pem": "service.pem",
    "fig": "service.fig.json",
    "routeUrl": "http://localhost:5432",
    "routePrefix": "/db",
    "keepFullPath": false
  }
}
```

**Request flow:**
```
Client -> /proxy/alias/myapp/query/users
  -> Peer: libp2p://12D3Koo.../router/08011220def.../query/users
  -> Backend: http://localhost:5432/db
```

### Example 4: Microservice with path transformation

Your auth service runs on `http://localhost:9000` and expects paths like `/auth/login`, `/auth/logout`, etc.

**Configuration:**
```json
{
  "auth-service": {
    "directory": "auth-service",
    "pem": "service.pem",
    "fig": "service.fig.json",
    "routeUrl": "http://localhost:9000",
    "routePrefix": "/auth",
    "keepFullPath": true
  }
}
```

**Request flow:**
```
Client -> /proxy/alias/myapp/login
  -> Peer: libp2p://12D3Koo.../router/08011220ghi.../login
  -> Backend: http://localhost:9000/auth/login
```

## How It Works

### 1. Service Registration

When a node starts, it reads `services.json` and:
1. Loads service keys from PEM files
2. Registers routes in the route table using service key hex as identifier
3. Stores routing configuration (routePrefix, keepFullPath) for each service key

### 2. Incoming Proxy Request

When a proxy request arrives:
1. Client sends request to `/proxy/alias/{alias}/{path}`
2. Proxy performs hierarchical path matching to find the appropriate service key
3. Proxy constructs libp2p URL: `libp2p://{peerID}/router/{serviceKey}/{path}`
4. Request is forwarded to the peer over libp2p

### 3. Request Routing on Receiving Peer

When the peer receives the request at `/router/{serviceKey}/{path}`:
1. Looks up the service key in the route table to get `routeUrl`
2. Retrieves routing configuration (routePrefix, keepFullPath)
3. Applies routing transformation based on configuration
4. Forwards the transformed request to the backend service

## Event Logging

The system emits events for debugging and monitoring:

### Service Route Registered Event
```json
{
  "type": "service_route_registered",
  "data": {
    "alias": "my-api",
    "serviceKey": "08011220abc...",
    "routeUrl": "http://localhost:8080",
    "routePrefix": "/api/v1",
    "keepFullPath": true
  }
}
```

### Proxy Request Event
```json
{
  "type": "proxy_request",
  "data": {
    "target": "http://localhost:8080/api/v1/users",
    "identifier": "08011220abc...",
    "method": "GET",
    "incomingPath": "/users",
    "targetPath": "/api/v1/users",
    "routePrefix": "/api/v1",
    "keepFullPath": true
  }
}
```

## Migration Guide

### Existing Services

Existing services without routing configuration will continue to work as before. The incoming path is forwarded as-is to the backend.

### Adding Routing Configuration

To add routing configuration to an existing service:

1. Add `routeUrl` field with your backend URL
2. Optionally add `routePrefix` if you need path transformation
3. Set `keepFullPath` to control path handling
4. Restart the node to load the new configuration

## Best Practices

1. **Use `keepFullPath: true`** for most services to preserve the request path
2. **Use `routePrefix`** when your backend expects a specific path prefix
3. **Use `keepFullPath: false`** only for services with a single fixed endpoint
4. **Test routing configuration** by checking event logs for path transformations
5. **Document your routing configuration** in service README files

## Troubleshooting

### Request returns 404

**Possible causes:**
1. Backend service is not running at the configured `routeUrl`
2. Path transformation is incorrect (check `routePrefix` and `keepFullPath`)
3. Backend service doesn't have the expected endpoint

**Solution:** Check event logs for the actual target URL being used

### Path is incorrect

**Possible causes:**
1. `keepFullPath` is set incorrectly
2. `routePrefix` is missing or incorrect

**Solution:** Review the routing behavior table above and adjust configuration

### Service key not found

**Possible causes:**
1. Service key not registered in route table
2. `routeUrl` not specified in services.json

**Solution:** Ensure `routeUrl` is configured and check service_route_registered events

