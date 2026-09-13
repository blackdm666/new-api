import assert from "node:assert/strict";
import { webcrypto } from "node:crypto";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { Miniflare, convertV4MiniflareOptions } from "miniflare";

import { handleRequest } from "../src/index.js";

if (!globalThis.crypto) globalThis.crypto = webcrypto;

const encoder = new TextEncoder();

class MediaBucket {
  constructor() { this.objects = new Map(); this.writes = 0; }
  async head(key) { return this.objects.get(key) || null; }
  async get(key, options) {
    const object = this.objects.get(key);
    if (!object) return null;
    const { offset = 0, length = object.size } = options?.range || {};
    return { ...object, body: new Blob([object.bytes.slice(offset, offset + length)]).stream() };
  }
  async put(key, stream, options) {
    assert.equal(options.onlyIf.etagDoesNotMatch, "*");
    if (this.objects.has(key)) { await stream.cancel(); return null; }
    const bytes = new Uint8Array(await new Response(stream).arrayBuffer());
    assert.deepEqual(new Uint8Array(await crypto.subtle.digest("SHA-256", bytes)), new Uint8Array(options.sha256));
    const object = { key, bytes, size: bytes.length, uploaded: new Date(), httpEtag: '"media-etag"', httpMetadata: options.httpMetadata };
    this.objects.set(key, object);
    this.writes++;
    return object;
  }
}

async function mediaUploadFixture(overrides = {}) {
  const secret = "test-media-secret-with-at-least-32-characters";
  const bytes = encoder.encode("0000ftypisom00000000video-bytes");
  const key = "reference-media/aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee.mp4";
  const b64 = (value) => Buffer.from(value).toString("base64url");
  const permit = { key, size: bytes.length, mime_type: "video/mp4", expires: Math.floor(Date.now() / 1000) + 600, sha256: b64(await crypto.subtle.digest("SHA-256", bytes)), ...overrides };
  const body = b64(JSON.stringify(permit));
  const cryptoKey = await crypto.subtle.importKey("raw", encoder.encode(secret), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  const signature = b64(await crypto.subtle.sign("HMAC", cryptoKey, encoder.encode(`media-upload.v1.${body}`)));
  const url = `https://worker.example/media/${key}`;
  return { key, url, bytes, env: { TRANSFER_SECRET: secret, VIDEO_BUCKET: new MediaBucket(), PUBLIC_MEDIA_ENABLED: "true" }, request: () => new Request(url, { method: "PUT", headers: { "X-Media-Upload-Token": `${body}.${signature}`, "Content-Type": permit.mime_type, "Content-Length": String(bytes.length) }, body: bytes }) };
}

test("real workerd R2 binding streams validated media and rejects a digest mismatch", async () => {
  const fixture = await mediaUploadFixture();
  const runtime = new Miniflare(convertV4MiniflareOptions({ name: "media-test", modules: ["index.js", "media.js"].map((name) => ({ type: "ESModule", path: fileURLToPath(new URL(`../src/${name}`, import.meta.url)) })), compatibilityDate: "2026-08-27", bindings: { TRANSFER_SECRET: fixture.env.TRANSFER_SECRET, PUBLIC_MEDIA_ENABLED: "true" }, r2Buckets: ["VIDEO_BUCKET"] }));
  try {
    const request = fixture.request();
    const uploaded = await runtime.dispatchFetch(fixture.url, { method: "PUT", headers: request.headers, body: fixture.bytes });
    assert.equal(uploaded.status, 201, await uploaded.text());
    const ranged = await runtime.dispatchFetch(fixture.url, { headers: { Range: "bytes=4-7" } });
    assert.equal(ranged.status, 206);
    assert.equal(await ranged.text(), "ftyp");
    const modifiedTime = (await runtime.dispatchFetch(fixture.url, { method: "HEAD" })).headers.get("X-Media-Expires-At");
    const retry = await runtime.dispatchFetch(fixture.url, { method: "PUT", headers: request.headers, body: fixture.bytes });
    assert.equal(retry.status, 409);
    assert.equal((await runtime.dispatchFetch(fixture.url, { method: "HEAD" })).headers.get("X-Media-Expires-At"), modifiedTime);
    const other = await mediaUploadFixture({ key: "reference-media/ffffffff-bbbb-4ccc-8ddd-eeeeeeeeeeee.mp4", sha256: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" });
    const otherURL = fixture.url.replace("aaaaaaaa-", "ffffffff-");
    const bad = await runtime.dispatchFetch(otherURL, { method: "PUT", headers: other.request().headers, body: other.bytes });
    assert.equal(bad.status, 502);
    assert.equal((await runtime.dispatchFetch(otherURL)).status, 404);
  } finally { await runtime.dispose(); }
});

test("media upload is scoped, write-once and anonymous reads support HEAD/ranges/expiry", async () => {
  const fixture = await mediaUploadFixture();
  const response = await handleRequest(fixture.request(), fixture.env, fixedLengthDependencies);
  assert.equal(response.status, 201, await response.text());
  assert.equal(fixture.env.VIDEO_BUCKET.writes, 1);
  const again = await handleRequest(fixture.request(), fixture.env, fixedLengthDependencies);
  assert.equal(again.status, 409);
  assert.equal(fixture.env.VIDEO_BUCKET.writes, 1);
  for (const [method, range, status, expected] of [["GET", null, 200, fixture.bytes.length], ["HEAD", null, 200, 0], ["GET", "bytes=0-3", 206, 4], ["GET", "bytes=-4", 206, 4]]) {
    const r = await handleRequest(new Request(fixture.url, { method, headers: range ? { Range: range } : {} }), fixture.env);
    assert.equal(r.status, status);
    assert.equal(r.headers.get("Access-Control-Allow-Origin"), "*");
    assert.equal(r.headers.get("Cache-Control"), "no-store");
    assert.equal((await r.arrayBuffer()).byteLength, expected);
  }
  const invalidRange = await handleRequest(new Request(fixture.url, { headers: { Range: "bytes=999999-" } }), fixture.env);
  assert.equal(invalidRange.status, 416);
  const expired = await handleRequest(new Request(fixture.url), fixture.env, { nowMilliseconds: Date.now() + 31 * 86400000 });
  assert.equal(expired.status, 410);
  assert.equal(fixture.env.VIDEO_BUCKET.writes, 1);
  fixture.env.VIDEO_BUCKET.objects.clear();
  assert.equal((await handleRequest(new Request(fixture.url), fixture.env)).status, 404);
  assert.equal(fixture.env.VIDEO_BUCKET.writes, 1);
});

test("media rejects unscoped credentials, expired grants, wrong type, oversize and unrelated objects", async () => {
  const fixture = await mediaUploadFixture();
  assert.equal((await handleRequest(new Request(fixture.url, { method: "PUT", body: fixture.bytes }), fixture.env)).status, 401);
  const expired = await mediaUploadFixture({ expires: Math.floor(Date.now() / 1000) - 1 });
  assert.equal((await handleRequest(expired.request(), expired.env)).status, 401);
  const oversized = await mediaUploadFixture({ size: 100_000_001 });
  assert.equal((await handleRequest(oversized.request(), oversized.env)).status, 401);
  const wrongType = fixture.request();
  wrongType.headers.set("Content-Type", "text/html");
  assert.equal((await handleRequest(wrongType, fixture.env)).status, 400);
  const badBytes = fixture.request();
  const forged = new Request(badBytes, { body: encoder.encode("<html>".padEnd(fixture.bytes.length, " ")) });
  assert.equal((await handleRequest(forged, fixture.env, fixedLengthDependencies)).status, 415);
  for (const path of ["invoices/private.pdf", "reference-media/list", "task-videos/../../private"]) {
    assert.equal((await handleRequest(new Request(`https://worker.example/media/${path}`), fixture.env)).status, 404);
  }
  assert.equal((await handleRequest(new Request(fixture.url, { method: "DELETE" }), fixture.env)).status, 405);
  assert.equal(fixture.env.VIDEO_BUCKET.writes, 0);
});

class TestFixedLengthStream {
  constructor(expectedLength) {
    let written = 0;
    const stream = new TransformStream({
      transform(chunk, controller) {
        written += chunk.byteLength;
        if (written > expectedLength)
          throw new TypeError("fixed-length stream received too many bytes");
        controller.enqueue(chunk);
      },
      flush() {
        if (written !== expectedLength)
          throw new TypeError("fixed-length stream received too few bytes");
      },
    });
    this.readable = stream.readable;
    this.writable = stream.writable;
  }
}

const fixedLengthDependencies = {
  FixedLengthStreamImpl: TestFixedLengthStream,
};

async function signature(secret, timestamp, body) {
  const key = await crypto.subtle.importKey(
    "raw",
    encoder.encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const signed = await crypto.subtle.sign(
    "HMAC",
    key,
    encoder.encode(`${timestamp}.${body}`),
  );
  return Array.from(new Uint8Array(signed), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
}

async function signedRequest(
  job,
  secret = "worker-test-secret",
  timestamp = 1787860000,
) {
  const body = JSON.stringify(job);
  return new Request("https://worker.example/transfer", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-NewAPI-Timestamp": String(timestamp),
      "X-NewAPI-Signature": await signature(secret, String(timestamp), body),
    },
    body,
  });
}

class MemoryBucket {
  constructor() {
    this.objects = new Map();
    this.putCount = 0;
  }

  async head(key) {
    return this.objects.get(key) || null;
  }

  async put(key, stream, options) {
    this.putCount += 1;
    const reader = stream.getReader();
    const chunks = [];
    let size = 0;
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(value);
      size += value.byteLength;
    }
    const object = {
      key,
      size,
      etag: `etag-${size}`,
      httpMetadata: options.httpMetadata,
      customMetadata: options.customMetadata,
      chunks,
    };
    this.objects.set(key, object);
    return object;
  }
}

function validJob(overrides = {}) {
  return {
    version: 1,
    task_id: "task_worker_transfer_123",
    source_url: "https://media.provider.example/video/result.mp4",
    key_prefix: `task-videos/2026/08/${"a".repeat(64)}`,
    max_bytes: 2 * 1024 * 1024,
    ...overrides,
  };
}

test("streams a signed public video into the bound R2 bucket", async () => {
  const bucket = new MemoryBucket();
  const request = await signedRequest(validJob());
  const response = await handleRequest(
    request,
    {
      TRANSFER_SECRET: "worker-test-secret",
      VIDEO_BUCKET: bucket,
    },
    {
      nowSeconds: 1787860000,
      ...fixedLengthDependencies,
      fetchImpl: async () =>
        new Response(encoder.encode("video-bytes"), {
          status: 200,
          headers: { "Content-Type": "video/mp4", "Content-Length": "11" },
        }),
    },
  );

  assert.equal(response.status, 200);
  const result = await response.json();
  assert.equal(result.success, true);
  assert.equal(result.reused, false);
  assert.equal(result.size, 11);
  assert.match(result.key, /^task-videos\/2026\/08\/[a-f0-9]{64}\.mp4$/);
  assert.equal(bucket.putCount, 1);
  assert.equal(
    (await bucket.head(result.key)).customMetadata.taskId,
    "task_worker_transfer_123",
  );
});

test("rejects an invalid signature before fetching the source", async () => {
  const request = await signedRequest(validJob(), "different-secret");
  let fetched = false;
  const response = await handleRequest(
    request,
    {
      TRANSFER_SECRET: "worker-test-secret",
      VIDEO_BUCKET: new MemoryBucket(),
    },
    {
      nowSeconds: 1787860000,
      fetchImpl: async () => {
        fetched = true;
        return new Response("unexpected");
      },
    },
  );

  assert.equal(response.status, 401);
  assert.equal(fetched, false);
  assert.equal((await response.json()).code, "invalid_signature");
});

test("rejects private and Docker-only source hosts", async () => {
  for (const sourceURL of [
    "http://sub2api:8080/v1/videos/id/content",
    "http://127.0.0.1/video.mp4",
    "http://192.168.1.5/video.mp4",
  ]) {
    const request = await signedRequest(validJob({ source_url: sourceURL }));
    const response = await handleRequest(
      request,
      {
        TRANSFER_SECRET: "worker-test-secret",
        VIDEO_BUCKET: new MemoryBucket(),
      },
      { nowSeconds: 1787860000 },
    );
    assert.equal(response.status, 400);
    assert.equal((await response.json()).code, "private_source_url");
  }
});

test("rejects a video whose declared size exceeds the signed limit", async () => {
  const request = await signedRequest(validJob());
  const response = await handleRequest(
    request,
    {
      TRANSFER_SECRET: "worker-test-secret",
      VIDEO_BUCKET: new MemoryBucket(),
    },
    {
      nowSeconds: 1787860000,
      fetchImpl: async () =>
        new Response(encoder.encode("video"), {
          status: 200,
          headers: {
            "Content-Type": "video/mp4",
            "Content-Length": String(3 * 1024 * 1024),
          },
        }),
    },
  );

  assert.equal(response.status, 413);
  assert.equal((await response.json()).code, "video_too_large");
});

test("falls back before upload when the source omits a fixed length", async () => {
  const bucket = new MemoryBucket();
  const request = await signedRequest(validJob({ max_bytes: 1024 * 1024 }));
  const response = await handleRequest(
    request,
    {
      TRANSFER_SECRET: "worker-test-secret",
      VIDEO_BUCKET: bucket,
    },
    {
      nowSeconds: 1787860000,
      fetchImpl: async () =>
        new Response(new Uint8Array(1024 * 1024 + 1), {
          status: 200,
          headers: { "Content-Type": "video/mp4" },
        }),
    },
  );

  assert.equal(response.status, 502);
  assert.equal((await response.json()).code, "source_length_required");
  assert.equal(bucket.putCount, 0);
});

test("validates every redirect target before following it", async () => {
  const request = await signedRequest(validJob());
  let calls = 0;
  const response = await handleRequest(
    request,
    {
      TRANSFER_SECRET: "worker-test-secret",
      VIDEO_BUCKET: new MemoryBucket(),
    },
    {
      nowSeconds: 1787860000,
      fetchImpl: async () => {
        calls += 1;
        return new Response(null, {
          status: 302,
          headers: { Location: "http://127.0.0.1/video.mp4" },
        });
      },
    },
  );

  assert.equal(response.status, 400);
  assert.equal((await response.json()).code, "private_source_url");
  assert.equal(calls, 1);
});

test("reuses an existing deterministic object without a second write", async () => {
  const bucket = new MemoryBucket();
  const dependencies = {
    nowSeconds: 1787860000,
    ...fixedLengthDependencies,
    fetchImpl: async () =>
      new Response(encoder.encode("video-bytes"), {
        status: 200,
        headers: { "Content-Type": "video/mp4", "Content-Length": "11" },
      }),
  };

  const first = await handleRequest(
    await signedRequest(validJob()),
    {
      TRANSFER_SECRET: "worker-test-secret",
      VIDEO_BUCKET: bucket,
    },
    dependencies,
  );
  const second = await handleRequest(
    await signedRequest(validJob()),
    {
      TRANSFER_SECRET: "worker-test-secret",
      VIDEO_BUCKET: bucket,
    },
    dependencies,
  );

  assert.equal(first.status, 200);
  assert.equal(second.status, 200);
  assert.equal((await second.json()).reused, true);
  assert.equal(bucket.putCount, 1);
});
