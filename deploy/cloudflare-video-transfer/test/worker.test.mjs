import assert from "node:assert/strict";
import { webcrypto } from "node:crypto";
import test from "node:test";

import { handleRequest } from "../src/index.js";

if (!globalThis.crypto) globalThis.crypto = webcrypto;

const encoder = new TextEncoder();

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

test("aborts a streaming upload when the source omits length and exceeds the limit", async () => {
  const request = await signedRequest(validJob({ max_bytes: 1024 * 1024 }));
  const response = await handleRequest(
    request,
    {
      TRANSFER_SECRET: "worker-test-secret",
      VIDEO_BUCKET: new MemoryBucket(),
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

  assert.equal(response.status, 413);
  assert.equal((await response.json()).code, "video_too_large");
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
