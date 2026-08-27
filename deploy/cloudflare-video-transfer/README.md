# NewAPI Cloudflare video transfer Worker

This Worker moves anonymously readable public video results directly from the
provider to the private `88api-videos` R2 bucket. Video bytes do not pass through
the NewAPI VPS.

The NewAPI backend signs every request with `TRANSFER_SECRET`. The Worker only
accepts public HTTP(S) sources, validates redirects, MIME type and size, writes
through a `ReadableStream`, and returns the deterministic R2 object key after a
successful `head` verification.

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
