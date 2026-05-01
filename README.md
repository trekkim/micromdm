# MicroMDM — Trend Micro Fork (v1.14.x)

This is a fork of [MicroMDM](https://github.com/micromdm/micromdm) maintained by Trend Micro IT.  
Original project by [@groob](https://github.com/groob) and the [MicroMDM contributors](https://github.com/micromdm/micromdm/graphs/contributors).

> **Note:** Upstream MicroMDM v1 is in maintenance mode (support ends end of 2025). This fork extends it with Managed Apple ID (MAID) sign-in support and ACME device attestation infrastructure.

---

## What This Fork Adds

| Version | Feature |
|---|---|
| v1.14.1 | `GetToken` MDM check-in handler + `-get-token-local` flag for local MAID JWT signing |
| v1.14.2 | `com.apple.mdm.token` added to dynamically generated enrollment profiles |
| v1.14.3 | `-acme-backend` reverse proxy for NanoCA ACME endpoint |
| v1.14.4 | ACME enrollment profile generation (`-acme-enrollment` flag) — serves SCEP+ACME dual payload profile at `/mdm/enroll` |
| v1.14.5 | Fix `VerifyCertificateMiddleware` to allow ACME certs on `Authenticate` — enables NanoCA-issued certs to register without being in SCEP depot |
| v1.14.6 | Fix `VerifyCertificateMiddleware` for all message types — bypass `HasCN` entirely when `validateSCEPIssuer=false`, rely on `UDIDCertAuthMiddleware` for security |

### GetToken / Managed Apple ID Sign-in

macOS devices enrolled in MDM can sign in with Managed Apple IDs (federated via Microsoft Entra ID) using the MDM server as a device identity provider.

When a user attempts to sign in with a Managed Apple ID, macOS sends a `GetToken` check-in message to the MDM server. The server responds with a signed JWT that Apple uses to attest the device identity. This fork implements this flow directly inside MicroMDM using the DEP private key already stored in BoltDB — **no external services required**.

### ACME Proxy (`-acme-backend`)

Proxies all `/acme/*` requests to a local [NanoCA](#nanoca) instance. This enables hardware-attested device identity certificates (Apple Managed Device Attestation) to be issued alongside standard SCEP certificates — without requiring nginx or any other reverse proxy.

---

## Building

Requires Go 1.25+.

```bash
# Build for current platform
make build

# Build for Linux (amd64)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
  -ldflags "-X github.com/micromdm/go4/version.version=v1.14.6" \
  -o build/linux/micromdm ./cmd/micromdm
```

---

## New Flags

### `-get-token-local`

Enable local MAID JWT signing using the DEP private key stored in MicroMDM's BoltDB.

```
-get-token-local=true
# env: GET_TOKEN_LOCAL=true
```

Prerequisites:
- DEP tokens must be configured in MicroMDM (`mdmctl apply dep-tokens`)
- Enrollment profile must include `com.apple.mdm.token` in `ServerCapabilities`
- The Managed Apple ID domain must be configured in Apple Business Manager (ABM)

### `-acme-backend`

URL of the NanoCA backend to proxy ACME requests to.

```
-acme-backend=https://localhost:9003
# env: ACME_BACKEND=https://localhost:9003
```

When set, MicroMDM proxies all `/acme/*` requests to the specified backend. The enrollment profile's `DirectoryURL` should point to `https://<your-mdm-server>/acme/directory`.

### `-acme-enrollment`

When combined with `-acme-backend`, makes MicroMDM serve a dual SCEP+ACME enrollment profile at `/mdm/enroll`. The MDM identity certificate uses the ACME cert (hardware-bound, Secure Enclave). SCEP cert is also included as fallback. Requires `-acme-backend` to be set.

```
-acme-enrollment=true
# env: ACME_ENROLLMENT=true
```

When enabled, the enrollment profile served at `/mdm/enroll` contains:
- `com.apple.security.scep` payload (backward compat)
- `com.apple.security.acme` payload (`HardwareBound: true`, `Attest: true`, `ECSECPrimeRandom` 256-bit)
- MDM payload with `IdentityCertificateUUID` pointing to ACME cert

### `-get-token-url` (alternative to `-get-token-local`)

URL of an external service (e.g. [NanoDEP](https://github.com/micromdm/nanodep)) to call for GetToken JWT generation.

```
-get-token-url=http://localhost:9001/v1/maidjwt/myserver
# env: GET_TOKEN_URL=...
```

---

## Full Systemd Example

```ini
[Unit]
Description=MicroMDM MDM Server
After=network.target

[Service]
ExecStart=/usr/local/bin/micromdm serve \
    -server-url=https://mdm.example.com \
    -api-key=YOUR_API_KEY \
    -filerepo /usr/local/mdm/pkg/ \
    -tls-cert /etc/ssl/mdm/fullchain.pem \
    -tls-key /etc/ssl/mdm/privkey.pem \
    -homepage=false \
    -command-webhook-url http://127.0.0.1:3001/webhook \
    -scep-client-validity 1460 \
    -dm http://127.0.0.1:9002 \
    -no-command-history=true \
    -get-token-local=true \
    -acme-backend=https://localhost:9003 \
    -acme-enrollment=true

Restart=on-failure

[Install]
WantedBy=multi-user.target
```

---

## Enrollment Profile Requirements

For Managed Apple ID sign-in to work, the MDM enrollment profile must include `com.apple.mdm.token` in `ServerCapabilities`:

```xml
<key>ServerCapabilities</key>
<array>
    <string>com.apple.mdm.per-user-connections</string>
    <string>com.apple.mdm.bootstraptoken</string>
    <string>com.apple.mdm.token</string>
</array>
```

Dynamically generated profiles (served at `/mdm/enroll`) include this automatically in v1.14.2+. If you serve a custom signed profile, add it manually.

---

## NanoCA

[NanoCA](https://github.com/brandonweeks/nanoca) is a lightweight ACME Certificate Authority with Apple Managed Device Attestation support, authored by [@brandonweeks](https://github.com/brandonweeks).

This repo includes a standalone NanoCA binary (built from the original source with a Go 1.25 compatibility patch) alongside MicroMDM for ACME device certificate enrollment. NanoCA is now integrated into the production deployment — it is no longer future work.

| Version | Feature |
|---|---|
| v1.0.0 | Initial release — ACME with Apple device-attest-01, Apple Enterprise Attestation Root CA embedded |

### What NanoCA Does

- Implements the ACME protocol (RFC 8555) with the `device-attest-01` challenge
- Verifies Apple hardware attestations using Apple's Enterprise Attestation Root CA
- Issues device identity certificates with hardware-attested serial number and UDID as SAN extensions
- Populates macOS `akd`'s **attestation map** with a `cert attestation`, enabling the `GetToken` MDM flow for seamless Managed Apple ID sign-in (no Safari redirect)

### NanoCA Build

```bash
cd nanoca-main
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
  -ldflags "-X main.Version=v1.0.0" \
  -o build/nanoca ./cmd/nanoca
```

### NanoCA Setup

```bash
# Generate Root CA (EC P-256, 10 years)
openssl ecparam -name prime256v1 -genkey -noout -out rootCA.key
openssl pkcs8 -topk8 -nocrypt -in rootCA.key -out rootCA-pkcs8.key
openssl req -new -x509 -days 3650 \
  -key rootCA-pkcs8.key \
  -out rootCA.crt \
  -subj "/O=Your Org/CN=MDM Device CA" \
  -addext "basicConstraints=critical,CA:TRUE"

# Run NanoCA
./nanoca \
  -ca-cert rootCA.crt \
  -ca-key rootCA-pkcs8.key \
  -base-url https://mdm.example.com \
  -listen :9003 \
  -storage /var/lib/nanoca/data
```

### NanoCA Systemd

```ini
[Unit]
Description=NanoCA ACME Certificate Authority
After=network.target

[Service]
ExecStart=/usr/local/bin/nanoca \
    -ca-cert /usr/local/mdm/nanoca/pki/rootCA.crt \
    -ca-key /usr/local/mdm/nanoca/pki/rootCA-pkcs8.key \
    -base-url https://mdm.example.com \
    -prefix /acme \
    -listen :9003 \
    -storage /usr/local/mdm/nanoca/data

Restart=on-failure

[Install]
WantedBy=multi-user.target
```

### ACME Enrollment Profile

```xml
<dict>
    <key>PayloadType</key>
    <string>com.apple.security.acme</string>
    <key>DirectoryURL</key>
    <string>https://mdm.example.com/acme/directory</string>
    <key>ClientIdentifier</key>
    <string>$SERIALNUMBER</string>
    <key>KeyType</key>
    <string>ECSECPrimeRandom</string>
    <key>KeySize</key>
    <integer>256</integer>
    <key>Attest</key>
    <true/>
    <key>HardwareBound</key>
    <true/>
    <key>PayloadIdentifier</key>
    <string>com.example.mdm.enroll.acme</string>
    <key>PayloadType</key>
    <string>com.apple.security.acme</string>
    <key>PayloadUUID</key>
    <string><!-- generate new UUID --></string>
    <key>PayloadVersion</key>
    <integer>1</integer>
</dict>
```

---

## Architecture Overview

```
macOS device
    │
    ├─ MDM check-in / connect  ──────────────► MicroMDM :443
    │                                              │
    ├─ GetToken (MAID sign-in) ──────────────► MicroMDM :443
    │   Apple expects JWT                          │ (-get-token-local)
    │   ◄─ signed JWT ────────────────────────────┘ (DEP private key)
    │
    ├─ /mdm/enroll (enrollment profile) ─────► MicroMDM :443
    │   (-acme-enrollment=true)                    │
    │   ◄─ dual SCEP+ACME profile ───────────────┘
    │       ├─ com.apple.security.scep  (fallback / backward compat)
    │       ├─ com.apple.security.acme (HardwareBound, Attest, P-256)
    │       └─ MDM payload → IdentityCertificateUUID → ACME cert
    │
    ├─ SCEP enrollment ──────────────────────► MicroMDM :443 /mdm/scep
    │   ◄─ SCEP identity certificate ───────────────┘
    │
    ├─ ACME enrollment ───────────────────────► MicroMDM :443 /acme/*
    │   device-attest-01 challenge                  │ (-acme-backend proxy)
    │                                           NanoCA :9003
    │   ◄─ hardware-attested certificate ───────────┘
    │       (Secure Enclave key, serial + UDID in SAN)
    │
    └─ Managed Apple ID sign-in
        Option A: PSSO + Entra ID web flow (Safari redirect) ✓ WORKING
        Option B: GetToken seamless (no redirect) — requires Apple-signed cert
            → blocked: deviceenrollment.apple.com not accessible for 3rd party MDM
```

---

## Known Limitations

### ACME cert attestation (akd attestation map)

NanoCA validates Apple device attestation via `device-attest-01` challenge but issues certificates signed by **our custom Root CA** (`Trend Micro MDM Device CA`). macOS `akd` daemon requires the MDM identity certificate to be signed by **Apple's Enterprise Attestation Sub CA** for the attestation map to be populated.

Without the attestation map entry, `akd` skips the `GetToken` MDM flow and falls back to browser-based Managed Apple ID sign-in via PSSO/Entra ID.

**Current workaround:** PSSO + Microsoft Entra ID provides Managed Apple ID sign-in via Safari redirect — fully functional for production use.

**Future path:** Apple's ACME endpoint `https://deviceenrollment.apple.com/acme/enrollment` issues Apple-signed attested certificates. Access to this endpoint for third-party MDM servers is currently not available / returned "not found" — this may change in future Apple releases.

---

## Credits

- **MicroMDM** — original MDM server: [github.com/micromdm/micromdm](https://github.com/micromdm/micromdm)  
  Authors: [@groob](https://github.com/groob) and contributors
- **NanoCA** — ACME CA with Apple device attestation: [github.com/brandonweeks/nanoca](https://github.com/brandonweeks/nanoca)  
  Author: [@brandonweeks](https://github.com/brandonweeks)
- **NanoDEP** — DEP proxy with MAID JWT support: [github.com/micromdm/nanodep](https://github.com/micromdm/nanodep)  
  Authors: micromdm team

This fork is maintained by **Trend Micro IT** for internal macOS fleet management.
