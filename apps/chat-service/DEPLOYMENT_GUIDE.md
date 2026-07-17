# Banyan Chat App - Deployment & Testing Guide

This guide walks through deploying a cluster of Banyan chat nodes using Docker and testing the full functionality.

## Prerequisites

- Docker 20.10+
- Docker Compose 2.0+
- OpenSSL (for key operations)
- curl, jq
- Node.js 18+ (for local testing)

## Phase 1: Build Images

```bash
# Navigate to docker directory
cd apps/chat-service/docker

# Build all images
./build-images.sh

# Verify images were created
docker images | grep banyan-chat
```

Expected output:
```
banyan-chat-static   latest    ...   ...
banyan-chat-ledger   latest    ...   ...
banyan-chat-mod      latest    ...   ...
banyan-chat-admin    latest    ...   ...
```

## Phase 2: Initialize Keys

```bash
cd apps/chat-service/topology

# Initialize fresh keys (if needed)
./keys.sh init

# Verify keys were generated
./keys.sh list

# Expected output:
# Authority Keys:
#   chat-authority: 302a300506032b...
# Service Keys:
#   chat-static-key: 302a300506032b...
#   chat-ledger-key: 302a300506032b...
#   chat-mod-key: 302a300506032b...
#   chat-admin-key: 302a300506032b...
```

## Phase 3: Deploy Cluster

```bash
cd apps/chat-service/docker

# Start all nodes
docker-compose up -d

# Check status
docker-compose ps

# Expected output:
# NAME            IMAGE                   STATUS    PORTS
# chat-static-1   banyan-chat-static      Up        0.0.0.0:8081->8080/tcp
# chat-ledger-1   banyan-chat-ledger      Up        0.0.0.0:8082->8080/tcp
# chat-ledger-2   banyan-chat-ledger      Up        0.0.0.0:8083->8080/tcp
# chat-mod-1      banyan-chat-mod         Up        0.0.0.0:8084->8080/tcp
# chat-admin-1    banyan-chat-admin       Up        0.0.0.0:8085->8080/tcp

# View logs
docker-compose logs -f
```

## Phase 4: Health Checks

### 4.1 Check Static Node (Webapp)

```bash
# Should return HTML
curl -s http://localhost:8081/ | head -5

# Check posts endpoint (empty initially)
curl -s http://localhost:8081/posts | jq

# Expected: {"posts": [], "total": 0}

# Check channels endpoint (empty initially)
curl -s http://localhost:8081/channels | jq

# Expected: {"channels": [], "total": 0}
```

### 4.2 Check Ledger Node

```bash
# Ledger nodes should have /ledger endpoints
curl -s http://localhost:8082/posts | jq

# Health check
curl -s -o /dev/null -w "%{http_code}" http://localhost:8082/
# Expected: 200
```

### 4.3 Check All Nodes Are Up

```bash
# Test all endpoints
for port in 8081 8082 8083 8084 8085; do
    status=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:$port/)
    echo "Port $port: $status"
done

# Expected:
# Port 8081: 200
# Port 8082: 200
# Port 8083: 200
# Port 8084: 200
# Port 8085: 200
```

## Phase 5: Test Ledger Functionality

### 5.1 Create a Channel

```bash
# Create a channel via ledger node
curl -X POST http://localhost:8082/ledger/channel \
  -H "Content-Type: application/json" \
  -d '{
    "name": "general",
    "description": "General discussion"
  }' | jq

# Expected response:
# {
#   "success": true,
#   "channel": {
#     "id": "...",
#     "name": "general",
#     ...
#   }
# }
```

### 5.2 Create a Post

```bash
# Post a message to the channel
curl -X POST http://localhost:8082/ledger/post \
  -H "Content-Type: application/json" \
  -d '{
    "channelId": "general",
    "content": "Hello from the chat app!",
    "author": "test-user-001"
  }' | jq

# Expected response:
# {
#   "success": true,
#   "entry": {
#     "id": "...",
#     "type": "post",
#     ...
#   }
# }
```

### 5.3 Verify Post Propagated

```bash
# Check posts on static node (should sync from ledger)
curl -s http://localhost:8081/posts | jq

# Check posts on ledger node
curl -s http://localhost:8082/posts | jq

# Check posts on second ledger node (should have synced)
curl -s http://localhost:8083/posts | jq
```

### 5.4 Test Ledger Sync

```bash
# Get sync status from ledger-1
curl -s "http://localhost:8082/ledger/sync" | jq

# Should return entries list

# Manually trigger sync from ledger-2
curl -s "http://localhost:8083/ledger/sync?since=" | jq
```

## Phase 6: Test Moderation

### 6.1 Create a Post to Delete

```bash
# Create a post from another "user"
curl -X POST http://localhost:8082/ledger/post \
  -H "Content-Type: application/json" \
  -d '{
    "channelId": "general",
    "content": "This is spam content",
    "author": "spammer-001"
  }' | jq

# Note the post ID from response
```

### 6.2 Delete Post as Moderator

```bash
# Delete the post using mod node (port 8084)
curl -X POST http://localhost:8084/mod/delete \
  -H "Content-Type: application/json" \
  -d '{
    "targetId": "<POST_ID_FROM_ABOVE>",
    "reason": "Spam content"
  }' | jq

# Expected:
# {"success": true, ...}
```

### 6.3 Verify Deletion

```bash
# Post should be tombstoned
curl -s http://localhost:8081/posts | jq

# Should not show the deleted post
```

### 6.4 Test Ban

```bash
# Ban a user
curl -X POST http://localhost:8084/mod/ban \
  -H "Content-Type: application/json" \
  -d '{
    "targetPeerId": "spammer-001",
    "reason": "Repeated spam",
    "duration": 86400000
  }' | jq

# Expected:
# {"success": true, ...}
```

## Phase 7: Test Admin Functions

### 7.1 Check Admin Status

```bash
curl -s http://localhost:8085/admin/status | jq

# Expected:
# {
#   "nodeType": "admin",
#   "keys": ["static", "ledger", "mod", "admin"],
#   "uptime": ...,
#   "ledgerEntries": ...,
#   ...
# }
```

### 7.2 Issue a Key (if implemented)

```bash
curl -X POST http://localhost:8085/admin/issue-key \
  -H "Content-Type: application/json" \
  -d '{
    "keyType": "ledger",
    "targetNode": "new-node-001",
    "expiresAt": 1711929600000
  }' | jq
```

## Phase 8: Test WebSocket

### 8.1 Connect to WebSocket

```bash
# Install wscat for testing
npm install -g wscat

# Connect to static node events
wscat -c ws://localhost:8081/events

# In another terminal, create a post
curl -X POST http://localhost:8082/ledger/post \
  -H "Content-Type: application/json" \
  -d '{
    "channelId": "general",
    "content": "WebSocket test message",
    "author": "ws-tester"
  }'

# The wscat terminal should receive:
# {"type": "post:new", "data": {...}}
```

### 8.2 Test Multiple Subscribers

```bash
# Open multiple terminals with wscat connections
# Terminal 1:
wscat -c ws://localhost:8081/events

# Terminal 2:
wscat -c ws://localhost:8082/events

# Terminal 3: Create a post
curl -X POST http://localhost:8082/ledger/post \
  -H "Content-Type: application/json" \
  -d '{
    "channelId": "general",
    "content": "Multi-subscriber test",
    "author": "test-user"
  }'

# Both terminals should receive the event
```

## Phase 9: Test Node-to-Node Sync

### 9.1 Verify Ledger Sync Between Nodes

```bash
# Get latest hash from ledger-1
HASH1=$(curl -s http://localhost:8082/ledger/sync | jq -r '.latestHash')

# Get latest hash from ledger-2
HASH2=$(curl -s http://localhost:8083/ledger/sync | jq -r '.latestHash')

# They should match (or be close)
echo "Ledger-1 hash: $HASH1"
echo "Ledger-2 hash: $HASH2"

# Get entry counts
COUNT1=$(curl -s http://localhost:8082/posts | jq '.total')
COUNT2=$(curl -s http://localhost:8083/posts | jq '.total')

echo "Ledger-1 entries: $COUNT1"
echo "Ledger-2 entries: $COUNT2"
```

### 9.2 Test Sync After Partition

```bash
# Stop ledger-2
docker-compose stop ledger-2

# Create posts on ledger-1
curl -X POST http://localhost:8082/ledger/post \
  -H "Content-Type: application/json" \
  -d '{
    "channelId": "general",
    "content": "Post during partition",
    "author": "test-user"
  }'

# Restart ledger-2
docker-compose start ledger-2

# Wait for sync (30 seconds default)
sleep 30

# Verify ledger-2 caught up
curl -s http://localhost:8083/posts | jq
```

## Phase 10: Test Key Rotation

### 10.1 Rotate a Key

```bash
cd apps/chat-service/topology

# Rotate the ledger key
./keys.sh rotate chat-ledger-key

# Restart nodes to pick up new keys
cd ../docker
docker-compose restart ledger-1 ledger-2 mod-1 admin-1
```

### 10.2 Verify Old Key Invalidated

```bash
# Try to post with old key (should fail if node restarted with new key)
# The nodes should now only accept requests signed with the new key
```

## Phase 11: Load Testing

### 11.1 Create Multiple Posts

```bash
# Create 100 posts
for i in {1..100}; do
  curl -X POST http://localhost:8082/ledger/post \
    -H "Content-Type: application/json" \
    -d "{
      \"channelId\": \"general\",
      \"content\": \"Load test message $i\",
      \"author\": \"load-tester\"
    }" &
done
wait

# Check total count
curl -s http://localhost:8082/posts | jq '.total'
```

### 11.2 Check Sync Performance

```bash
# Time how long sync takes
time curl -s http://localhost:8083/ledger/sync > /dev/null
```

## Phase 12: Cleanup

```bash
# Stop all containers
docker-compose down

# Remove volumes (clears all data)
docker-compose down -v

# Remove images
docker rmi banyan-chat-static banyan-chat-ledger banyan-chat-mod banyan-chat-admin

# Clean up generated keys (if needed)
cd ../topology
rm -rf output/
```

## Troubleshooting

### Container won't start

```bash
# Check logs
docker-compose logs ledger-1

# Check if port is in use
lsof -i :8082

# Rebuild image
docker-compose build ledger-1
```

### Posts not syncing

```bash
# Check sync endpoints
curl -v http://localhost:8082/ledger/sync
curl -v http://localhost:8083/ledger/sync

# Check environment variables
docker-compose exec ledger-1 env | grep SYNC

# Check network connectivity
docker-compose exec ledger-1 ping ledger-2
```

### WebSocket not connecting

```bash
# Check if WebSocket endpoint is responding
curl -v -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Key: test" -H "Sec-WebSocket-Version: 13" \
  http://localhost:8081/events

# Check container logs for WebSocket errors
docker-compose logs static-1 | grep -i websocket
```

### Key rotation issues

```bash
# Verify keys exist
ls -la apps/chat-service/topology/output/keys/

# Check key mapping
cat apps/chat-service/topology/output/keys/key-mapping.json | jq

# Regenerate if needed
./keys.sh init
```

## Success Criteria

The deployment is successful if:

- [ ] All 5 containers are running and healthy
- [ ] Static node serves the webapp at port 8081
- [ ] Can create channels via ledger node
- [ ] Can create posts via ledger node
- [ ] Posts appear on all nodes within 30 seconds
- [ ] Moderator can delete posts
- [ ] Moderator can ban users
- [ ] Admin status endpoint returns valid data
- [ ] WebSocket clients receive real-time events
- [ ] Ledger sync works between nodes
- [ ] Key rotation can be performed
- [ ] Nodes recover after restart

## Next Steps

After successful deployment:

1. **Monitor** - Set up logging aggregation
2. **Scale** - Add more static nodes for load balancing
3. **Secure** - Enable TLS, configure firewall rules
4. **Backup** - Set up persistent volume backups
5. **Alert** - Configure health check alerts
