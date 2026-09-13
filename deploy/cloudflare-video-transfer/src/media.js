const RETENTION_MS = 30 * 24 * 60 * 60 * 1000;
const MAX_UPLOAD_BYTES = 100_000_000;
const referenceKey = /^reference-media\/[a-f0-9-]{36}\.(mp4|webm|mov|mp3|wav|ogg|m4a|flac|png|jpg|webp|gif)$/;
const videoKey = /^task-videos\/[0-9]{4}\/[0-9]{2}\/[a-f0-9]{64}\.(mp4|webm|mov)$/;
const types = {
  mp4: "video/mp4", webm: "video/webm", mov: "video/quicktime",
  mp3: "audio/mpeg", wav: "audio/wav", ogg: "audio/ogg", m4a: "audio/mp4", flac: "audio/flac",
  png: "image/png", jpg: "image/jpeg", webp: "image/webp", gif: "image/gif",
};

function mediaHeaders() {
  return new Headers({
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, HEAD, PUT, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type, X-Media-Upload-Token, Range, If-Range",
    "Access-Control-Expose-Headers": "Content-Length, Content-Range, Accept-Ranges, ETag, X-Media-Expires-At",
    "Cache-Control": "no-store",
    "X-Content-Type-Options": "nosniff",
    "Referrer-Policy": "no-referrer",
    "Content-Security-Policy": "default-src 'none'; sandbox",
  });
}

function failure(status, message, headers = mediaHeaders()) {
  headers.set("Content-Type", "application/json");
  return new Response(JSON.stringify({ error: { message } }), { status, headers });
}

function fromBase64URL(text) {
  if (!/^[A-Za-z0-9_-]+$/.test(text)) throw new Error("invalid base64url");
  return Uint8Array.from(atob(text.replace(/-/g, "+").replace(/_/g, "/")), (c) => c.charCodeAt(0));
}

async function uploadPermit(request, env, key, now) {
  const ticket = request.headers.get("X-Media-Upload-Token") || "";
  if (ticket.length > 2048 || !env.TRANSFER_SECRET) return null;
  const parts = ticket.split(".");
  if (parts.length !== 2) return null;
  try {
    const signingKey = await crypto.subtle.importKey("raw", new TextEncoder().encode(env.TRANSFER_SECRET), { name: "HMAC", hash: "SHA-256" }, false, ["verify"]);
    if (!await crypto.subtle.verify("HMAC", signingKey, fromBase64URL(parts[1]), new TextEncoder().encode(`media-upload.v1.${parts[0]}`))) return null;
    const permit = JSON.parse(new TextDecoder().decode(fromBase64URL(parts[0])));
    if (permit.key !== key || !referenceKey.test(key) || !Number.isSafeInteger(permit.size) || permit.size <= 0 || permit.size > MAX_UPLOAD_BYTES ||
        !Number.isSafeInteger(permit.expires) || permit.expires * 1000 <= now || permit.expires * 1000 > now + 11 * 60 * 1000 ||
        permit.mime_type !== types[key.split(".").at(-1)] || fromBase64URL(permit.sha256).length !== 32) return null;
    return permit;
  } catch {
    return null;
  }
}

// Validate container signatures without buffering the media in Worker memory.
// R2 independently checks the digest and FixedLengthStream checks the byte count.
function matchesMediaSignature(bytes, extension) {
  const starts = (...values) => values.every((value, index) => bytes[index] === value);
  const text = (start, end) => new TextDecoder("latin1").decode(bytes.subarray(start, end));
  switch (extension) {
    case "mp4": case "m4a": return text(4, 8) === "ftyp";
    case "mov": return ["ftyp", "moov", "wide", "mdat"].includes(text(4, 8));
    case "webm": return starts(0x1a, 0x45, 0xdf, 0xa3);
    case "png": return starts(137, 80, 78, 71, 13, 10, 26, 10);
    case "jpg": return starts(255, 216, 255);
    case "gif": return ["GIF87a", "GIF89a"].includes(text(0, 6));
    case "webp": return text(0, 4) === "RIFF" && text(8, 12) === "WEBP";
    case "wav": return text(0, 4) === "RIFF" && text(8, 12) === "WAVE";
    case "ogg": return text(0, 4) === "OggS";
    case "flac": return text(0, 4) === "fLaC";
    case "mp3": return text(0, 3) === "ID3" || (bytes[0] === 255 && (bytes[1] & 0xe0) === 0xe0);
    default: return false;
  }
}

async function uploadMedia(request, env, key, now, dependencies) {
  const permit = await uploadPermit(request, env, key, now);
  if (!permit) return failure(401, "Invalid or expired upload authorization");
  if (Number(request.headers.get("Content-Length")) !== permit.size || request.headers.get("Content-Type") !== permit.mime_type || !request.body) {
    return failure(400, "Media length and content type must match the upload authorization");
  }
  const existing = await env.VIDEO_BUCKET.head(key);
  if (existing) return failure(409, "Media already uploaded; reuse its public URL");
  const reader = request.body.getReader();
  const chunks = [];
  let prefixLength = 0;
  while (prefixLength < 12) {
    const chunk = await reader.read();
    if (chunk.done) break;
    chunks.push(chunk.value);
    prefixLength += chunk.value.byteLength;
  }
  const prefix = new Uint8Array(Math.min(12, prefixLength));
  let written = 0;
  for (const chunk of chunks) {
    const part = chunk.subarray(0, prefix.length - written);
    prefix.set(part, written);
    written += part.length;
  }
  if (!matchesMediaSignature(prefix, key.split(".").at(-1))) {
    await reader.cancel();
    return failure(415, "Media signature does not match its content type");
  }
  const FixedStream = dependencies.FixedLengthStreamImpl || globalThis.FixedLengthStream;
  const stream = new FixedStream(permit.size);
  const writer = stream.writable.getWriter();
  const pump = (async () => {
    try {
      for (const chunk of chunks) await writer.write(chunk);
      for (;;) {
        const chunk = await reader.read();
        if (chunk.done) break;
        await writer.write(chunk.value);
      }
      await writer.close();
    } catch (error) {
      await writer.abort(error).catch(() => {});
      throw error;
    } finally {
      await reader.cancel().catch(() => {});
    }
  })();
  const put = env.VIDEO_BUCKET.put(key, stream.readable, {
    onlyIf: { etagDoesNotMatch: "*" },
    sha256: fromBase64URL(permit.sha256).buffer,
    httpMetadata: { contentType: permit.mime_type },
    customMetadata: { uploadedBy: "reference-media", sha256: permit.sha256 },
  }).then(async (stored) => {
    if (!stored) await stream.readable.cancel("object already exists").catch(() => {});
    return stored;
  });
  const [, stored] = await Promise.all([pump, put]);
  if (!stored) return failure(409, "Media already uploaded; reuse its public URL");
  const headers = mediaHeaders();
  headers.set("Content-Type", "application/json");
  return new Response(JSON.stringify({ url: request.url, size: stored.size, mime_type: permit.mime_type, expires_at: Math.floor((stored.uploaded.getTime() + RETENTION_MS) / 1000) }), { status: 201, headers });
}

function requestedRange(value, size) {
  if (!value) return undefined;
  const match = /^bytes=(\d*)-(\d*)$/.exec(value);
  if (!match || (!match[1] && !match[2])) return null;
  const start = match[1] ? Number(match[1]) : Math.max(0, size - Number(match[2]));
  const end = match[1] && match[2] ? Math.min(size - 1, Number(match[2])) : size - 1;
  if (!Number.isSafeInteger(start) || !Number.isSafeInteger(end) || start < 0 || start >= size || end < start || (!match[1] && Number(match[2]) <= 0)) return null;
  return { offset: start, length: end - start + 1 };
}

export async function handleMediaRequest(request, env, dependencies = {}) {
  const url = new URL(request.url);
  const key = url.pathname.slice("/media/".length);
  const headers = mediaHeaders();
  if (env.PUBLIC_MEDIA_ENABLED !== "true" || !env.VIDEO_BUCKET || (!referenceKey.test(key) && !videoKey.test(key))) return failure(404, "Media not found");
  if (url.search) return failure(400, "Media URLs do not accept query parameters");
  if (request.method === "OPTIONS") return new Response(null, { status: 204, headers });
  const now = dependencies.nowMilliseconds ?? Date.now();
  try {
    if (request.method === "PUT") return await uploadMedia(request, env, key, now, dependencies);
    if (!["GET", "HEAD"].includes(request.method)) return failure(405, "Method not allowed");
    const metadata = await env.VIDEO_BUCKET.head(key);
    if (!metadata) return failure(404, "Media not found");
    const expires = metadata.uploaded.getTime() + RETENTION_MS;
    if (now >= expires) return failure(410, "Media expired; upload a new asset explicitly");
    headers.set("Content-Type", types[key.split(".").at(-1)]);
    headers.set("Content-Disposition", "inline");
    headers.set("Accept-Ranges", "bytes");
    headers.set("ETag", metadata.httpEtag);
    headers.set("X-Media-Expires-At", String(Math.floor(expires / 1000)));
    const ifRange = request.headers.get("If-Range");
    const range = request.method === "HEAD" || (ifRange && ifRange !== metadata.httpEtag) ? undefined : requestedRange(request.headers.get("Range"), metadata.size);
    if (range === null) {
      headers.set("Content-Range", `bytes */${metadata.size}`);
      return failure(416, "Invalid byte range", headers);
    }
    headers.set("Content-Length", String(range?.length ?? metadata.size));
    if (request.method === "HEAD") return new Response(null, { headers });
    const object = await env.VIDEO_BUCKET.get(key, range ? { range } : undefined);
    if (!object) return failure(404, "Media not found");
    if (range) headers.set("Content-Range", `bytes ${range.offset}-${range.offset + range.length - 1}/${metadata.size}`);
    return new Response(object.body, { status: range ? 206 : 200, headers });
  } catch {
    // Never return signed upload tickets, storage errors or source metadata.
    return failure(502, "Media storage request failed");
  }
}
