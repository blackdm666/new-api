const MODELS = ["wan3.0-video-480p", "wan3.0-video-720p", "wan3.0-video-1080p"];
const RATIOS = ["16:9", "9:16", "1:1", "4:3", "3:4"];

export const meta = {
  apiVersion: 1,
  key: "xinmeng-wan3",
  name: "XinMeng Wan3",
  version: "1.0.1",
  author: { name: "88API" },
  description: { en: "Wan3 video generation through XinMeng", zh: "通过 XinMeng 生成 Wan3 视频" },
  models: MODELS,
  fetchMode: "per_task",
  protocols: ["openai_video"],
  usageSchema: {
    seconds: { type: "number", unit: "second", description: { en: "Video generation unit price", zh: "视频生成单价" } },
  },
};

function object(value) { return value && typeof value === "object" && !Array.isArray(value) ? value : {}; }
function text(value) { return typeof value === "string" ? value.trim() : ""; }
function first() { for (const value of arguments) { if (text(value)) return text(value); } return ""; }
function firstArray() {
  for (const value of arguments) {
    if (Array.isArray(value) && value.length) return value.slice();
    if (text(value)) return [text(value)];
  }
  return [];
}
function secondsFor(req) {
  // The same validated value is sent upstream and supplied to both billing modes.
  const value = req.duration !== undefined ? req.duration : req.seconds !== undefined ? req.seconds : 5;
  if ((typeof value !== "number" && typeof value !== "string") || String(value).trim() === "") throw new Error("duration must be an integer between 4 and 30");
  const seconds = Number(value);
  if (!Number.isInteger(seconds) || seconds < 4 || seconds > 30) throw new Error("duration must be an integer between 4 and 30");
  if (req.duration !== undefined && req.seconds !== undefined && Number(req.seconds) !== seconds) throw new Error("duration and seconds must agree");
  return seconds;
}
function payloadFor(req, model) {
  if (!MODELS.includes(model)) throw new Error("unsupported Wan3 model");
  const metadata = object(req.metadata);
  const all = Object.assign({}, metadata, req);
  const sizeRatios = { "1280x720": "16:9", "1920x1080": "16:9", "2560x1440": "16:9", "720x1280": "9:16", "1080x1920": "9:16", "1440x2560": "9:16", "1024x1024": "1:1", "1440x1440": "1:1", "1920x1440": "4:3", "1440x1920": "3:4" };
  const body = {
    model: model,
    ratio: first(all.ratio, all.aspect_ratio, RATIOS.includes(req.size) ? req.size : sizeRatios[req.size], "16:9"),
    duration: secondsFor(req),
    resolution: model.slice(model.lastIndexOf("-") + 1),
  };
  const prompt = text(req.prompt);
  if (prompt) body.prompt = prompt;
  const negative = first(req.negative_prompt, metadata.negative_prompt);
  if (negative) body.negative_prompt = negative;
  const images = firstArray(req.images, req.image, req.input_reference, all.referenceImages, all.reference_images, all.image_urls, metadata.images, metadata.image, all.file_paths);
  const videos = firstArray(all.referenceVideos, all.reference_videos, all.video_urls, all.videos);
  const audios = firstArray(all.referenceAudios, all.reference_audios, all.audio_urls, all.audios);
  for (const entry of [["referenceImages", images], ["referenceVideos", videos], ["referenceAudios", audios]]) {
    if (entry[1].length) {
      for (const value of entry[1]) {
        if (!text(value) && !(entry[0] === "referenceImages" && object(value).__fileRef)) throw new Error(entry[0] + " must contain media references");
      }
      body[entry[0]] = entry[1];
    }
  }
  const firstFrame = first(all.firstFrame, all.first_frame, Array.isArray(all.first_image) ? all.first_image[0] : all.first_image);
  const lastFrame = first(all.lastFrame, all.last_frame, Array.isArray(all.last_image) ? all.last_image[0] : all.last_image);
  if (firstFrame) body.firstFrame = firstFrame;
  if (lastFrame) body.lastFrame = lastFrame;
  if (Array.isArray(all.media) && all.media.length) body.media = all.media.slice();
  if (all.seed !== undefined && all.seed !== null) {
    if (!Number.isSafeInteger(all.seed)) throw new Error("seed must be an integer");
    body.seed = all.seed;
  }
  if (Object.keys(object(all.camera_control)).length) body.camera_control = Object.assign({}, all.camera_control);
  if (all.generateAudio !== undefined || all.generate_audio !== undefined) throw new Error("generateAudio is not supported by model " + model);
  if (!prompt && !images.length && !videos.length && !audios.length && !firstFrame && !lastFrame && !body.media) throw new Error("prompt or reference media is required");
  if (Array.from(prompt).length > 5000) throw new Error("prompt must contain at most 5000 characters");
  if (!RATIOS.includes(body.ratio)) throw new Error("unsupported aspect ratio");
  // Preserve the old static 720/1080 contract and the catalog-derived 480 contract.
  const is480 = body.resolution === "480p";
  if (images.length > (is480 ? 30 : 10) || videos.length > (is480 ? 10 : 5) || audios.length > (is480 ? 10 : 5)) throw new Error("too many media references");
  if (!is480 && (firstFrame || lastFrame) && (images.length || videos.length || audios.length)) throw new Error("first/last frame mode cannot be mixed with reference media");
  if (lastFrame && !firstFrame) throw new Error("lastFrame requires firstFrame");
  return body;
}

export function decodeRequest(ctx) {
  if (!ctx.body || !["json", "multipart"].includes(ctx.body.kind)) throw new Error("JSON or multipart body required");
  let req;
  if (ctx.body.kind === "json") {
    if (!ctx.body.value || typeof ctx.body.value !== "object" || Array.isArray(ctx.body.value)) throw new Error("JSON object required");
    req = Object.assign({}, ctx.body.value);
  } else {
    req = {};
    for (const key of Object.keys(ctx.body.fields || {})) {
      const values = ctx.body.fields[key];
      if (values.length !== 1) throw new Error(key + " must be provided once");
      req[key] = values[0];
    }
    for (const key of ["metadata", "images", "referenceImages", "reference_images", "referenceVideos", "reference_videos", "referenceAudios", "reference_audios", "media", "camera_control", "seed"]) {
      if (req[key] !== undefined) {
        try { req[key] = JSON.parse(req[key]); } catch (_error) { throw new Error(key + " must be valid JSON"); }
      }
    }
    const files = ctx.body.files || [];
    if (files.length > 1 || files.some(function (file) { return file.field !== "input_reference"; })) throw new Error("only one input_reference file is supported");
    if (files.length) {
      if (req.images || req.image || req.input_reference) throw new Error("input_reference file cannot be combined with image fields");
      req.images = [{ __fileRef: files[0].ref, encoding: "dataUrl" }];
    }
  }
  if (req.metadata !== undefined && (!req.metadata || typeof req.metadata !== "object" || Array.isArray(req.metadata))) throw new Error("metadata must be an object");
  req.model = ctx.model;
  const payload = payloadFor(req, ctx.model);
  req.duration = payload.duration;
  return { kind: "submit", model: ctx.model, action: payload.referenceImages || payload.referenceVideos || payload.referenceAudios || payload.firstFrame || payload.media ? "image_to_video" : "text_to_video", requestBody: req };
}

function base(ctx) { return String(ctx.baseUrl).replace(/\/+$/, "").replace(/\/v1$/, ""); }
export function buildSubmitRequest(ctx) {
  return { url: base(ctx) + "/v1/videos/generations", method: "POST", headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json" }, body: payloadFor(object(ctx.requestBody), ctx.upstreamModel || ctx.model) };
}
export function extractUsage(ctx) { return { seconds: secondsFor(object(ctx.requestBody)) }; }

function taskBody(value) {
  const body = object(value);
  return body.status !== undefined || body.id || body.task_id ? body : object(body.data);
}
function errorMessage(body) { return first(object(body.error).message, body.error, body.message, "video generation failed"); }
function resultURL(value, depth) {
  if ((depth || 0) > 4) return "";
  if (text(value)) return /^https?:\/\//i.test(text(value)) ? text(value) : "";
  if (Array.isArray(value)) { for (const item of value) { const url = resultURL(item, (depth || 0) + 1); if (url) return url; } return ""; }
  const body = object(value);
  for (const key of ["result", "video_url", "result_url", "url", "data"]) { const url = resultURL(body[key], (depth || 0) + 1); if (url) return url; }
  return "";
}
export function parseSubmitResponse(_ctx, response) {
  const body = taskBody(response.body);
  if (["failed", "cancelled", "expired"].includes(String(body.status).toLowerCase())) throw new Error(errorMessage(body));
  const id = first(body.id, body.task_id);
  if (!id) throw new Error("upstream task id is empty");
  return { taskId: id, taskData: response.body };
}
export function buildQueryRequest(ctx) {
  return { url: base(ctx) + "/v1/tasks/" + encodeURIComponent(ctx.taskId), method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey, Accept: "application/json" } };
}
export function parseTaskResult(_ctx, value) {
  const body = taskBody(value);
  const statuses = { pending: "QUEUED", queued: "QUEUED", submitted: "QUEUED", processing: "IN_PROGRESS", in_progress: "IN_PROGRESS", running: "IN_PROGRESS", completed: "SUCCESS", success: "SUCCESS", succeeded: "SUCCESS", done: "SUCCESS", failed: "FAILURE", cancelled: "FAILURE", expired: "FAILURE" };
  const status = statuses[String(body.status || "").trim().toLowerCase()];
  if (!status) return { status: "UNKNOWN", reason: "unrecognized XinMeng task status" };
  const terminal = status === "SUCCESS" || status === "FAILURE";
  const progress = Number(String(body.progress || "0").replace(/%$/, ""));
  const result = { status: status, progress: terminal ? "100%" : (Number.isFinite(progress) ? Math.max(0, Math.min(99, Math.floor(progress))) : 0) + "%" };
  if (status === "SUCCESS") { const url = resultURL(body); if (url) result.url = url; }
  if (status === "FAILURE") result.reason = errorMessage(body);
  return result;
}
// XinMeng's existing Wan3 contract bills the requested duration. Completion
// keeps the host's frozen seconds fact; pointsCost/actualDuration are not prices.
export function listArtifacts(task) { return task.status === "SUCCESS" && resultURL(task.data) ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : []; }
export function buildContentRequest(ctx) {
  const url = resultURL(ctx.data);
  if (ctx.artifactKey !== "video" || !url) throw new Error("artifact_not_found");
  return { url: url, method: ctx.clientRequest.method, credentialless: true };
}
export const protocols = {
  openai_video: {
    decodeRequest: decodeRequest,
    render: function (_ctx, task) {
      const statuses = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
      const result = { id: task.task_id, task_id: task.task_id, object: "video", model: object(task.properties).origin_model_name || "", status: statuses[task.status] || "unknown", progress: Number(String(task.progress || "0").replace("%", "")), created_at: task.created_at };
      if (task.status === "SUCCESS" || task.status === "FAILURE") result.completed_at = task.updated_at;
      if (task.status === "FAILURE") result.error = { code: "video_generation_failed", message: task.fail_reason || "video generation failed" };
      return result;
    },
  },
};
