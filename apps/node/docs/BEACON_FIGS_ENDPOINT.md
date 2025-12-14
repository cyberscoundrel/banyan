# Beacon Figs Endpoint

## Overview

The `/services/beacons/figs` endpoint generates signed fig files for all active service beacons on the node. This is different from `/services/figs` which only lists static `.fig` files from the figs directory.

## Endpoint

**URL:** `POST /services/beacons/figs` or `GET /services/beacons/figs`

**Purpose:** Generate signed fig files for all active service beacons with request-specific nonce or requester peer ID.

## Request

### Method
- `POST` (recommended) - Allows passing nonce or requester peer ID in request body
- `GET` - Uses this node's peer ID as the requester

### Request Body (POST only)

```json
{
  "nonce": "999999999",
  "requesterPeerId": "12D3KooWABC..."
}
```

**Parameters:**
- `nonce` (optional): Hex-encoded nonce for replay protection. If provided, will be included in the signed fig.
- `requesterPeerId` (optional): Peer ID of the requester. If provided, will be included in the signed fig.

**Default Behavior:**
- If neither `nonce` nor `requesterPeerId` is provided, the endpoint will use **this node's peer ID** as the requester.

## Response

```json
{
  "peerId": "12D3KooWXYZ...",
  "services": [
    {
      "alias": "fig1",
      "keys": [
        {
          "key": "08021220abcdef...",
          "hash": "1234567890abcdef"
        }
      ],
      "fig": {
        "serviceAlias": "fig1",
        "root": {
          "path": "/",
          "keys": ["08021220abcdef..."],
          "children": [
            {
              "path": "/api",
              "keys": ["08021220xyz..."]
            }
          ]
        },
        "data": {},
        "expiresAt": "2025-11-14T12:00:00Z",
        "requesterPeerId": "12D3KooWXYZ...",
        "nonce": "999999999",
        "requiredSigners": ["authority1", "authority2"],
        "authoritySignatures": ["sig1...", "sig2..."],
        "signatures": ["newsig1...", "newsig2..."]
      }
    },
    {
      "alias": "fig2",
      "keys": [
        {
          "key": "08021220abcdef...",
          "hash": "1234567890abcdef"
        }
      ],
      "fig": {
        "serviceAlias": "fig2",
        "root": {
          "path": "/",
          "keys": ["08021220def..."]
        },
        "expiresAt": "2025-11-14T12:00:00Z",
        "requesterPeerId": "12D3KooWXYZ...",
        "nonce": "999999999",
        "signatures": ["newsig3..."]
      }
    }
  ],
  "time": "2025-10-14T11:48:42Z"
}
```

**Response Fields:**
- `peerId`: This node's peer ID
- `services`: Array of service entries (one per fig template, or one per beacon if no templates)
  - `alias`: Service alias from the fig template's `serviceAlias` field
  - `keys`: Array of public keys for this service beacon
    - `key`: Full hex-encoded public key
    - `hash`: First 8 bytes of SHA256 hash of the public key
  - `fig`: Signed fig file object
    - `serviceAlias`: Service alias from the fig template (NOT the beacon alias)
    - `root`: Root node structure from fig template (includes hierarchical paths and keys)
    - `data`: Optional metadata from fig template
    - `expiresAt`: Expiration timestamp from fig template
    - `requesterPeerId`: Requester peer ID (from request or this node's ID)
    - `nonce`: Nonce from request (if provided)
    - `requiredSigners`: Required authority signers from fig template
    - `authoritySignatures`: Pre-signed authority signatures from fig template
    - `signatures`: Service key signatures (includes both template signatures and new signatures)
- `time`: Response timestamp

**Important: Multiple Services Per Beacon**
- If a beacon has multiple fig templates configured (e.g., `fig1.json`, `fig2.json`), the endpoint will return **multiple service entries** - one for each template
- Each service entry will have its own `serviceAlias` from the template
- All service entries for the same beacon will share the same `keys` array
- This allows a single service beacon to advertise multiple service aliases with different configurations

**Note on Fig Templates:**
- Fig templates configured in `services.json` are loaded and used to generate signed figs
- Each template generates a separate service entry in the response
- The `serviceAlias` in the response comes from the template's `serviceAlias` field
- The `root` structure (including hierarchical paths and keys) is taken from the template
- If a beacon has no templates, a single basic fig is generated using the beacon's alias

## Examples

### Example 1: Using a nonce

```bash
curl -X POST http://localhost:8080/services/beacons/figs \
  -H "Content-Type: application/json" \
  -d '{"nonce":"999999999"}'
```

### Example 2: Using a requester peer ID

```bash
curl -X POST http://localhost:8080/services/beacons/figs \
  -H "Content-Type: application/json" \
  -d '{"requesterPeerId":"12D3KooWABC123..."}'
```

### Example 3: Using this node's peer ID (default)

```bash
curl -X GET http://localhost:8080/services/beacons/figs
```

or

```bash
curl -X POST http://localhost:8080/services/beacons/figs \
  -H "Content-Type: application/json" \
  -d '{}'
```

## Comparison with Other Endpoints

### `/services/figs` (GET only)
- Lists static `.fig` files from the figs directory
- Shows alias cache and active searches
- Does NOT generate signed figs for beacons

### `/services/beacons/figs` (GET/POST) - **NEW**
- Generates signed figs for all active service beacons
- Accepts nonce or requester peer ID
- Returns fully signed fig files ready for validation

### `/services/fig/{serviceKey}` (GET/POST)
- Generates signed fig for a specific service key
- Requires service key in URL path
- Accepts nonce or requester peer ID

## Use Cases

1. **Testing beacon signatures**: Verify that your service beacons are properly signing fig files
2. **Exporting service configurations**: Get all signed figs for active services in one call
3. **Service discovery**: Retrieve all services this node is advertising with their signed credentials
4. **Integration testing**: Validate the complete fig signing flow including authority signatures

## Security Notes

- The endpoint respects fig templates configured on beacons, including:
  - Authority signatures (pre-signed by governance/CA keys)
  - Required signers
  - Expiration times
  - Custom metadata
- Nonce replay protection is enforced when validating received figs
- Signatures are generated using the service beacon's private key
- The endpoint is only accessible via the management server (localhost by default)

