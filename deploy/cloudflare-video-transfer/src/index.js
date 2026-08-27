const MAX_JOB_BODY_BYTES = 32 * 1024;
const MAX_CLOCK_SKEW_SECONDS = 5 * 60;
const MAX_REDIRECTS = 3;
const MAX_SINGLE_UPLOAD_BYTES = 5 * 1024 ** 3 - 5 * 1024 ** 2;
const VIDEO_EXTENSIONS = new Map([
  ["video/mp4", ".mp4"],
  ["video/webm", ".webm"],
  ["video/quicktime", ".mov"],
]);

class TransferError extends Error {
  constructor(status, code, message) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

function jsonResponse(body, status = 200) {
  return Response.json(body, {
    status,
    headers: {
      "Cache-Control": "no-store",
      "X-Content-Type-Options": "nosniff",
    },
  });
}

function bytesToHex(bytes) {
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join(
    "",
  );
}

async function signPayload(secret, timestamp, body) {
  const key = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const signature = await crypto.subtle.sign(
    "HMAC",
    key,
    new TextEncoder().encode(`${timestamp}.${body}`),
  );
  return bytesToHex(new Uint8Array(signature));
}

function constantTimeEqual(left, right) {
  if (left.length !== right.length || left.length === 0) return false;
  let difference = 0;
  for (let index = 0; index < left.length; index += 1) {
    difference |= left.charCodeAt(index) ^ right.charCodeAt(index);
  }
  return difference === 0;
}

function isPrivateIPv4(host) {
  const parts = host.split(".").map(Number);
  if (
    parts.length !== 4 ||
    parts.some((part) => !Number.isInteger(part) || part < 0 || part > 255)
  ) {
    return false;
  }
  return (
    parts[0] === 10 ||
    parts[0] === 127 ||
    (parts[0] === 169 && parts[1] === 254) ||
    (parts[0] === 172 && parts[1] >= 16 && parts[1] <= 31) ||
    (parts[0] === 192 && parts[1] === 168) ||
    parts[0] === 0
  );
}

function validateSourceURL(rawURL) {
  let parsed;
  try {
    parsed = new URL(rawURL);
  } catch {
    throw new TransferError(400, "invalid_source_url", "source URL is invalid");
  }
  if (
    !["http:", "https:"].includes(parsed.protocol) ||
    parsed.username ||
    parsed.password
  ) {
    throw new TransferError(
      400,
      "invalid_source_url",
      "source URL must be public HTTP(S)",
    );
  }
  const host = parsed.hostname
    .toLowerCase()
    .replace(/^\[/, "")
    .replace(/\]$/, "")
    .replace(/\.$/, "");
  if (
    !host ||
    host === "localhost" ||
    host.endsWith(".localhost") ||
    host.endsWith(".local") ||
    !host.includes(".") ||
    isPrivateIPv4(host) ||
    host.includes(":")
  ) {
    throw new TransferError(
      400,
      "private_source_url",
      "source URL is not publicly routable",
    );
  }
  parsed.hash = "";
  return parsed;
}

async function cancelBodyQuietly(body, reason) {
  if (!body) return;
  try {
    await body.cancel(reason);
  } catch {
    // The response is being abandoned deliberately; cancellation errors do
    // not change the transfer decision.
  }
}

function validateJob(job) {
  if (!job || job.version !== 1 || typeof job.task_id !== "string") {
    throw new TransferError(400, "invalid_job", "transfer job is invalid");
  }
  if (!/^task_[A-Za-z0-9_-]{8,191}$/.test(job.task_id)) {
    throw new TransferError(400, "invalid_task_id", "task ID is invalid");
  }
  if (!/^task-videos\/\d{4}\/\d{2}\/[a-f0-9]{64}$/.test(job.key_prefix || "")) {
    throw new TransferError(
      400,
      "invalid_object_key",
      "object key prefix is invalid",
    );
  }
  if (
    !Number.isSafeInteger(job.max_bytes) ||
    job.max_bytes < 1024 * 1024 ||
    job.max_bytes > MAX_SINGLE_UPLOAD_BYTES
  ) {
    throw new TransferError(
      400,
      "invalid_size_limit",
      "video size limit is invalid",
    );
  }
  validateSourceURL(job.source_url);
}

async function fetchSource(sourceURL, fetchImpl) {
  let currentURL = validateSourceURL(sourceURL);
  for (
    let redirectCount = 0;
    redirectCount <= MAX_REDIRECTS;
    redirectCount += 1
  ) {
    const response = await fetchImpl(currentURL, {
      method: "GET",
      redirect: "manual",
      headers: { Accept: "video/*, application/octet-stream;q=0.9" },
    });
    if ([301, 302, 303, 307, 308].includes(response.status)) {
      await cancelBodyQuietly(response.body, "following validated redirect");
      if (redirectCount === MAX_REDIRECTS) {
        throw new TransferError(
          502,
          "too_many_redirects",
          "video source redirected too many times",
        );
      }
      const location = response.headers.get("Location");
      if (!location)
        throw new TransferError(
          502,
          "invalid_redirect",
          "video source redirect is invalid",
        );
      currentURL = validateSourceURL(new URL(location, currentURL).toString());
      continue;
    }
    if (response.status !== 200 || !response.body) {
      throw new TransferError(
        502,
        "upstream_unavailable",
        `video source returned status ${response.status}`,
      );
    }
    return { response, finalURL: currentURL };
  }
  throw new TransferError(
    502,
    "upstream_unavailable",
    "video source is unavailable",
  );
}

function resolveVideoType(response, finalURL) {
  const contentType = (response.headers.get("Content-Type") || "")
    .split(";", 1)[0]
    .trim()
    .toLowerCase();
  if (VIDEO_EXTENSIONS.has(contentType))
    return { contentType, extension: VIDEO_EXTENSIONS.get(contentType) };
  if (contentType === "application/octet-stream" || contentType === "") {
    const path = finalURL.pathname.toLowerCase();
    if (path.endsWith(".webm"))
      return { contentType: "video/webm", extension: ".webm" };
    if (path.endsWith(".mov"))
      return { contentType: "video/quicktime", extension: ".mov" };
    if (path.endsWith(".mp4") || path.endsWith(".m4v"))
      return { contentType: "video/mp4", extension: ".mp4" };
  }
  throw new TransferError(
    415,
    "unexpected_content_type",
    "video source did not return supported video content",
  );
}

function createCountingStream(body, maxBytes, counter) {
  const reader = body.getReader();
  return new ReadableStream({
    async pull(controller) {
      const { done, value } = await reader.read();
      if (done) {
        controller.close();
        return;
      }
      counter.size += value.byteLength;
      if (counter.size > maxBytes) {
        try {
          await reader.cancel("video size limit exceeded");
        } catch {
          // The size violation remains authoritative even if cancellation fails.
        }
        controller.error(
          new TransferError(
            413,
            "video_too_large",
            "video source exceeded configured size limit",
          ),
        );
        return;
      }
      controller.enqueue(value);
    },
    cancel(reason) {
      return reader.cancel(reason);
    },
  });
}

export async function handleRequest(request, env, dependencies = {}) {
  if (
    request.method !== "POST" ||
    new URL(request.url).pathname !== "/transfer"
  ) {
    return jsonResponse(
      { success: false, code: "not_found", message: "not found" },
      404,
    );
  }
  if (!env.TRANSFER_SECRET || !env.VIDEO_BUCKET) {
    return jsonResponse(
      {
        success: false,
        code: "not_configured",
        message: "worker is not configured",
      },
      503,
    );
  }

  try {
    const declaredLength = Number(request.headers.get("Content-Length") || 0);
    if (declaredLength > MAX_JOB_BODY_BYTES) {
      throw new TransferError(
        413,
        "job_too_large",
        "transfer job is too large",
      );
    }
    const body = await request.text();
    if (new TextEncoder().encode(body).byteLength > MAX_JOB_BODY_BYTES) {
      throw new TransferError(
        413,
        "job_too_large",
        "transfer job is too large",
      );
    }
    const timestamp = request.headers.get("X-NewAPI-Timestamp") || "";
    const signature = (
      request.headers.get("X-NewAPI-Signature") || ""
    ).toLowerCase();
    const parsedTimestamp = Number(timestamp);
    const nowSeconds = dependencies.nowSeconds ?? Math.floor(Date.now() / 1000);
    if (
      !Number.isSafeInteger(parsedTimestamp) ||
      Math.abs(nowSeconds - parsedTimestamp) > MAX_CLOCK_SKEW_SECONDS
    ) {
      throw new TransferError(
        401,
        "expired_signature",
        "request signature expired",
      );
    }
    const expectedSignature = await signPayload(
      env.TRANSFER_SECRET,
      timestamp,
      body,
    );
    if (!constantTimeEqual(signature, expectedSignature)) {
      throw new TransferError(
        401,
        "invalid_signature",
        "request signature is invalid",
      );
    }

    let job;
    try {
      job = JSON.parse(body);
    } catch {
      throw new TransferError(
        400,
        "invalid_json",
        "transfer job is not valid JSON",
      );
    }
    validateJob(job);

    const fetchImpl = dependencies.fetchImpl || fetch;
    const { response, finalURL } = await fetchSource(job.source_url, fetchImpl);
    const { contentType, extension } = resolveVideoType(response, finalURL);
    const declaredVideoLength = Number(
      response.headers.get("Content-Length") || 0,
    );
    if (declaredVideoLength > job.max_bytes) {
      await cancelBodyQuietly(response.body, "video size limit exceeded");
      throw new TransferError(
        413,
        "video_too_large",
        "video source exceeded configured size limit",
      );
    }

    const key = `${job.key_prefix}${extension}`;
    const existing = await env.VIDEO_BUCKET.head(key);
    if (existing) {
      await cancelBodyQuietly(response.body, "object already exists");
      return jsonResponse({
        success: true,
        key,
        mime_type: existing.httpMetadata?.contentType || contentType,
        size: existing.size,
        etag: existing.etag,
        reused: true,
      });
    }

    const counter = { size: 0 };
    const stored = await env.VIDEO_BUCKET.put(
      key,
      createCountingStream(response.body, job.max_bytes, counter),
      {
        httpMetadata: { contentType },
        customMetadata: {
          taskId: job.task_id,
          transferredBy: "new-api-video-transfer",
        },
      },
    );
    if (!stored)
      throw new TransferError(
        502,
        "r2_write_failed",
        "R2 did not persist the video",
      );
    const verified = await env.VIDEO_BUCKET.head(key);
    if (!verified || verified.size !== counter.size || counter.size <= 0) {
      throw new TransferError(
        502,
        "r2_verify_failed",
        "R2 video verification failed",
      );
    }
    return jsonResponse({
      success: true,
      key,
      mime_type: contentType,
      size: counter.size,
      etag: verified.etag,
      reused: false,
    });
  } catch (error) {
    const transferError =
      error instanceof TransferError
        ? error
        : new TransferError(502, "transfer_failed", "video transfer failed");
    return jsonResponse(
      {
        success: false,
        code: transferError.code,
        message: transferError.message,
      },
      transferError.status,
    );
  }
}

export default {
  fetch(request, env) {
    return handleRequest(request, env);
  },
};
