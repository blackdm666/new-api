# NewAPI Cloudflare video transfer Worker

This Worker moves anonymously readable public video results directly from the
provider to the private `88api-videos` R2 bucket. Video bytes do not pass through
the NewAPI VPS.

The NewAPI backend signs every request with `TRANSFER_SECRET`. The Worker only
accepts public HTTP(S) sources, validates redirects, MIME type and size, writes
through a Cloudflare `FixedLengthStream`, and returns the deterministic R2
object key after a successful `head` verification. Sources without a trustworthy
`Content-Length`, with content encoding, or above the single-part R2 limit are
handed back to NewAPI's VPS fallback instead of being buffered in Worker memory.

`sub2api:8080`, private hosts and provider downloads that require special
credentials intentionally remain on the NewAPI VPS fallback path.

## Commands

```bash
npm install
npm test
npx wrangler deploy
npx wrangler secret put TRANSFER_SECRET
```

The R2 binding is declared in `wrangler.jsonc`. Never commit the transfer secret
or provider credentials.

## Public media delivery (explicit opt-in)

`PUBLIC_MEDIA_ENABLED=true` enables anonymous GET/HEAD (including byte ranges)
under `/media/task-videos/` and `/media/reference-media/` only. The bucket stays
private. Objects expire 30 days after their original upload; reads never renew,
copy or recreate them. Configure separate 30-day R2 lifecycle rules for both
prefixes; keep the existing incomplete-multipart cleanup rule. Public links are
bearer capabilities: anyone with a link can read it until deletion/expiry.

Deploy this Worker first, then set the NewAPI Compose environment:

```ini
TASK_MEDIA_PUBLIC_ENABLED=true
TASK_MEDIA_PUBLIC_BASE_URL=https://new-api-video-transfer.guodamao.workers.dev/media
```

Keep existing `TASK_VIDEO_CACHE_ENABLED=true`, S3/R2 storage, Worker transfer
settings and the matching `TASK_VIDEO_WORKER_SECRET` / `TRANSFER_SECRET`.
New video results are archived once before publishing success, even when the
provider URL was previously trusted for direct use. Authenticated video queries
return the public URL. `/v1/videos/{task_id}/content` redirects anonymously only
for successful, already archived videos. Task metadata and generation stay
authenticated. Disabling the NewAPI flag restores the prior authenticated
content route; disabling the Worker flag also revokes public media delivery.

### Standard reference-upload API

1. Compute the complete file's SHA-256, encoded as unpadded base64url.
2. Send authenticated `POST /v1/media/uploads` with JSON:
   `{"size":12345,"mime_type":"video/mp4","sha256":"<base64url SHA-256>"}`.
3. The response contains `url`, `upload_url`, `method: "PUT"`, `headers` and
   `expires_at` (the **10-minute upload-ticket expiry**, not object retention).
4. PUT the original file bytes to `upload_url`, using only the returned
   `Content-Type` and `X-Media-Upload-Token` headers. Do not forward the API key.
   Browsers set Content-Length for Blob/File bodies; other clients must send it.
5. After HTTP 201, verify anonymous HEAD/GET of `url`, then use it in the target
   model's existing reference field. `X-Media-Expires-At` reports object expiry.
   HTTP 409 means the same one-object ticket was already used; reuse the URL.

Uploads accept 1–100,000,000 bytes per file, with no per-day limit. Supported
MIME types: video/mp4, video/webm, video/quicktime, audio/mpeg, audio/wav,
audio/ogg, audio/mp4, audio/flac, image/png, image/jpeg, image/webp, image/gif.
The Worker verifies signed object scope, expiry, byte count, container signature
and SHA-256. Writes are conditional and cannot overwrite or refresh an object.
Expired media must be explicitly uploaded again; automatic retries must not
request a new ticket merely to renew retention. This service transports media;
it does not transcode formats or override individual model capability limits.

Verification: Node test runner includes real Miniflare/workerd R2 PUT, digest
rejection, ranges, duplicate writes, expiry, and private-prefix rejection.
Security references reviewed: OWASP ASVS 5.0.0, Authentication, Session
Management and File Upload Cheat Sheets. This scoped test suite is not a claim
of full ASVS compliance. Preserve the transfer secret and original Worker
version for rollback; do not remove existing lifecycle rules during rollout.
