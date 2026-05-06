# Transaction Signing Agent (TSA) — API Contract

The TSA is an external HTTP service that signs Stellar transactions on behalf of the SDP.
The TSA returns **raw signatures only** — never the full signed XDR — so the SDP retains
full control over transaction construction and network submission.
Private keys never leave the TSA.

## Authentication

All endpoints (except `GET /health`) require a Bearer token:

```
Authorization: Bearer <TSA_AUTH_TOKEN>
```

## Endpoints

### POST /sign

Sign a single transaction.

**Request**
```json
{
  "transaction_xdr": "AAAA...",
  "network_passphrase": "Public Global Stellar Network ; September 2015",
  "signers": ["GABCD...", "GEFGH..."]
}
```

**Response 200 OK**
```json
{
  "signatures": [
    {
      "public_key": "GABCD...",
      "signature": "<base64-encoded-ed25519-signature>"
    }
  ]
}
```

**Error codes**

| Code | Meaning | SDP behavior |
|------|---------|--------------|
| 400 | Malformed request or unknown signer | Terminal — will not retry |
| 401 | Invalid or missing auth token | Terminal |
| 500 | TSA internal error | Retryable — SDP retries next cycle |
| timeout | TSA unreachable | Retryable |

---

### POST /sign/batch

Sign multiple transactions in one request. Use this for performance when the SDP
processes many payments in parallel.

Returns `200` if all succeeded, `207 Multi-Status` if some failed.

**Request**
```json
{
  "transactions": [
    {
      "transaction_xdr": "AAAA...",
      "network_passphrase": "Public Global Stellar Network ; September 2015",
      "signers": ["GABCD..."]
    },
    {
      "transaction_xdr": "BBBB...",
      "network_passphrase": "Public Global Stellar Network ; September 2015",
      "signers": ["GEFGH..."]
    }
  ]
}
```

**Response 200 OK** (all succeeded)
```json
{
  "results": [
    { "status": 200, "signatures": [{ "public_key": "GABCD...", "signature": "..." }] },
    { "status": 200, "signatures": [{ "public_key": "GEFGH...", "signature": "..." }] }
  ]
}
```

**Response 207 Multi-Status** (partial failure)
```json
{
  "results": [
    { "status": 200, "signatures": [{ "public_key": "GABCD...", "signature": "..." }] },
    { "status": 400, "error": "unknown signer public key" }
  ]
}
```

---

### GET /health

TSA readiness check. Returns available signer public keys.
No authentication required.

**Response 200 OK**
```json
{
  "status": "ok",
  "public_keys": ["GABCD...", "GEFGH..."]
}
```

---

## Design rationale

- The TSA returns **raw signatures**, not full signed XDR. This prevents the TSA from modifying transaction content (amount, destination, etc.) before signing.
- The TSA **never submits** to the Stellar network. The SDP retains that responsibility.
- The **batch endpoint** exists for performance: the SDP processes many payments in parallel.
- **mTLS** is recommended between SDP and TSA in production. The TSA should be on a private network only.
