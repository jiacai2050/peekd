const state = {
  files: new Map(),
  labels: new WeakMap(),
  fileSources: new WeakMap(),
  current: null,
  requestID: 0,
  worker: null,
  pending: new Map(),
  objectURL: null,
  mermaidPromise: null,
  parser: {phase: "loading", error: null},
  source: {phase: "idle", token: 0},
};

const $ = (id) => document.getElementById(id);
const fileInput = $("file-input");
const directoryInput = $("directory-input");
const fileList = $("file-list");
const preview = $("preview");
const filter = $("filter");
const urlInput = $("url-input");
const maxRemoteFileSize = 128 * 1024 * 1024;
const maxPreviewBytes = 512 * 1024 + 1;
const sourceStoreName = "sources";
let databasePromise;
let storageQueue = Promise.resolve();

function beginSourceOperation(phase) {
  state.source = {phase, token: state.source.token + 1};
  return state.source.token;
}

function sourceOperationCurrent(token) {
  return state.source.token === token;
}

function finishSourceOperation(token) {
  if (sourceOperationCurrent(token)) state.source.phase = "idle";
}

function enqueueStorageTask(task) {
  const next = storageQueue.then(task, task);
  storageQueue = next.catch(() => {});
  return next;
}

function openDatabase() {
  if (databasePromise) return databasePromise;
  databasePromise = new Promise((resolve, reject) => {
    if (!window.indexedDB) {
      reject(new Error("IndexedDB is unavailable."));
      return;
    }
    const request = indexedDB.open("peekd-local", 1);
    request.onupgradeneeded = () => {
      request.result.createObjectStore(sourceStoreName, {keyPath: "key"});
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error || new Error("Unable to open local storage."));
  });
  return databasePromise;
}

async function deleteSavedSourceNow(key) {
  const database = await openDatabase();
  const transaction = database.transaction(sourceStoreName, "readwrite");
  transaction.objectStore(sourceStoreName).delete(key);
  await new Promise((resolve, reject) => {
    transaction.oncomplete = resolve;
    transaction.onerror = () => reject(transaction.error || new Error("Unable to remove saved source."));
  });
}

function databaseRequest(request) {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error || new Error("Local storage request failed."));
  });
}

async function saveSource(source) {
  const database = await openDatabase();
  const transaction = database.transaction(sourceStoreName, "readwrite");
  transaction.objectStore(sourceStoreName).put({...source, updatedAt: Date.now()});
  await new Promise((resolve, reject) => {
    transaction.oncomplete = resolve;
    transaction.onerror = () => reject(transaction.error || new Error("Unable to save source."));
  });
}

async function saveSourceBestEffort(source) {
  const token = state.source.token;
  try {
    await enqueueStorageTask(async () => {
      if (!sourceOperationCurrent(token)) return;
      await saveSource(source);
    });
  } catch (error) {
    setStatus(`Unable to save ${source.name || "source"} for later: ${error.message}`, true);
  }
}

function deleteSavedSource(key) {
  return enqueueStorageTask(() => deleteSavedSourceNow(key));
}

async function listSources() {
  const database = await openDatabase();
  const transaction = database.transaction(sourceStoreName, "readonly");
  return databaseRequest(transaction.objectStore(sourceStoreName).getAll());
}

async function clearSavedSources() {
  const database = await openDatabase();
  const transaction = database.transaction(sourceStoreName, "readwrite");
  transaction.objectStore(sourceStoreName).clear();
  await new Promise((resolve, reject) => {
    transaction.oncomplete = resolve;
    transaction.onerror = () => reject(transaction.error || new Error("Unable to clear saved sources."));
  });
}

function setStatus(message, isError = false) {
  const status = $("status");
  status.textContent = message;
  status.classList.toggle("error", isError);
}

function failWorker(error) {
  state.parser = {phase: "failed", error};
  for (const pending of state.pending.values()) pending.reject(error);
  state.pending.clear();
  setStatus(error.message, true);
}

function displayName(file) {
  return state.labels.get(file) || file._peekdPath || file.webkitRelativePath || file.name;
}

function addFiles(files, selectFirst = false) {
  for (const file of files) {
    const originalName = file._peekdPath || file.webkitRelativePath || file.name;
    const extensionIndex = originalName.lastIndexOf(".");
    const extension = extensionIndex > 0 ? originalName.slice(extensionIndex) : "";
    const stem = extension ? originalName.slice(0, extensionIndex) : originalName;
    let name = originalName;
    let suffix = 2;
    while (state.files.has(name) && state.files.get(name) !== file) {
      name = `${stem} (${suffix++})${extension}`;
    }
    state.labels.set(file, name);
    state.files.set(name, file);
  }
  renderFileList();
  if (!state.current && state.files.size) selectFile([...state.files.values()][0]);
  else if (selectFirst && files.length) selectFile(files[0]);
}

function remoteFileName(url, contentDisposition) {
  const dispositionName = contentDisposition && contentDisposition.match(/filename\*?=(?:UTF-8'')?"?([^";]+)"?/i);
  if (dispositionName) {
    try { return decodeURIComponent(dispositionName[1]); } catch (_) { return dispositionName[1]; }
  }
  const path = new URL(url).pathname;
  const name = path.split("/").filter(Boolean).pop();
  return name || "remote-file";
}

async function loadURL(url, {persist = true, isCurrent = () => true} = {}) {
  let parsed;
  try {
    parsed = new URL(url);
  } catch (_) {
    throw new Error("Enter a valid URL.");
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    throw new Error("Only http:// and https:// URLs are supported.");
  }

  setStatus("Downloading URL…");
  let response;
  try {
    response = await fetch(parsed.href, {credentials: "omit"});
  } catch (_) {
    throw new Error("Unable to fetch this URL. The server may block browser CORS requests.");
  }
  if (!response.ok) throw new Error(`URL returned HTTP ${response.status}.`);
  const contentLength = Number(response.headers.get("content-length"));
  if (Number.isFinite(contentLength) && contentLength > maxRemoteFileSize) {
    throw new Error("Remote file is larger than the 128 MiB browser limit.");
  }
  const blob = await response.blob();
  if (blob.size > maxRemoteFileSize) {
    throw new Error("Remote file is larger than the 128 MiB browser limit.");
  }
  if (!isCurrent()) return false;
  const file = new File([blob], remoteFileName(parsed.href, response.headers.get("content-disposition")), {
    type: response.headers.get("content-type")?.split(";")[0] || blob.type,
    lastModified: Date.now(),
  });
  const source = {key: `url:${parsed.href}`, kind: "url", url: parsed.href, name: file.name};
  state.fileSources.set(file, source);
  if (persist) await saveSourceBestEffort(source);
  if (!isCurrent()) return false;
  addFiles([file]);
  if (await selectFile(file)) setStatus("Ready");
  return true;
}

function renderFileList() {
  const query = filter.value.trim().toLowerCase();
  const files = [...state.files.entries()]
    .filter(([name]) => name.toLowerCase().includes(query))
    .sort(([a], [b]) => a.localeCompare(b, undefined, {numeric: true}));
  $("file-count").textContent = `${state.files.size} file${state.files.size === 1 ? "" : "s"}`;
  fileList.replaceChildren();
  if (!files.length) {
    const empty = document.createElement("div");
    empty.className = "empty-state";
    empty.textContent = state.files.size ? "No matching files." : "Open a file or directory.";
    fileList.append(empty);
    return;
  }
  for (const [name, file] of files) {
    const row = document.createElement("div");
    row.className = "file-row";
    row.classList.toggle("selected", state.current === file);
    row.setAttribute("role", "button");
    row.tabIndex = 0;
    row.title = name;
    const icon = document.createElement("span");
    icon.className = "file-icon";
    icon.textContent = iconFor(file.name);
    const label = document.createElement("span");
    label.className = "file-name";
    label.textContent = name;
    const remove = document.createElement("button");
    remove.className = "file-remove";
    remove.type = "button";
    remove.title = `Remove ${name}`;
    remove.setAttribute("aria-label", `Remove ${name}`);
    remove.textContent = "×";
    remove.addEventListener("click", (event) => {
      event.stopPropagation();
      removeFile(file);
    });
    remove.addEventListener("keydown", (event) => event.stopPropagation());
    row.append(icon, label, remove);
    row.addEventListener("click", () => selectFile(file));
    row.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        selectFile(file);
      }
    });
    fileList.append(row);
  }
}

function removeFile(file) {
  const token = beginSourceOperation("loading");
  for (const [name, item] of state.files) {
    if (item === file) state.files.delete(name);
  }
  const source = state.fileSources.get(file);
  state.fileSources.delete(file);
  state.labels.delete(file);
  if (source && source.kind !== "directory") {
    deleteSavedSource(source.key)
      .catch((error) => setStatus(error.message, true))
      .finally(() => finishSourceOperation(token));
  } else if (source) {
    source.excludedPaths = [...new Set([...(source.excludedPaths || []), file._peekdPath || file.name])];
    saveSourceBestEffort(source).finally(() => finishSourceOperation(token));
  } else {
    finishSourceOperation(token);
  }
  if (state.current === file) {
    state.current = null;
    if (state.objectURL) {
      URL.revokeObjectURL(state.objectURL);
      state.objectURL = null;
    }
    preview.replaceChildren(message("Select a file to preview"));
  }
  renderFileList();
}

function iconFor(name) {
  const extension = name.toLowerCase().split(".").pop();
  if (["png", "jpg", "jpeg", "gif", "svg", "webp", "avif"].includes(extension)) return "🖼️";
  if (["mp3", "wav", "ogg", "flac", "m4a"].includes(extension)) return "🎵";
  if (["mp4", "webm", "mov", "mkv"].includes(extension)) return "🎬";
  if (["zip", "tar", "gz", "tgz"].includes(extension)) return "📦";
  if (["md", "markdown", "org"].includes(extension)) return "📝";
  if (["json", "xml", "csv", "tsv"].includes(extension)) return "🧾";
  if (extension === "pdf") return "📕";
  return "📄";
}

function directPreviewKind(file) {
  const type = file.type || "";
  if (type.startsWith("image/")) return "image";
  if (type.startsWith("audio/")) return "audio";
  if (type.startsWith("video/")) return "video";
  if (type === "application/pdf") return "pdf";

  const name = file.name.toLowerCase();
  const extension = name.includes(".") ? name.slice(name.lastIndexOf(".")) : "";
  if ([".avif", ".bmp", ".gif", ".ico", ".jpeg", ".jpg", ".png", ".svg", ".tif", ".tiff", ".webp"].includes(extension)) return "image";
  if ([".aac", ".flac", ".m4a", ".mp3", ".oga", ".ogg", ".opus", ".wav", ".weba"].includes(extension)) return "audio";
  if ([".avi", ".m4v", ".mkv", ".mov", ".mp4", ".ogv", ".webm"].includes(extension)) return "video";
  if (extension === ".pdf") return "pdf";
  return null;
}

function isArchive(file) {
  const name = file.name.toLowerCase();
  return file.type === "application/zip" ||
    file.type === "application/x-tar" ||
    name.endsWith(".zip") ||
    name.endsWith(".tar") ||
    name.endsWith(".tar.gz") ||
    name.endsWith(".tgz");
}

function startWorker() {
  state.worker = new Worker("worker.js");
  state.parser = {phase: "loading", error: null};
  state.worker.addEventListener("message", (event) => {
    const message = event.data;
    if (message.type === "ready") {
      state.parser = {phase: "ready", error: null};
      setStatus("Ready");
      if (state.current) void selectFile(state.current);
      return;
    }
    if (message.type === "error") {
      setStatus(message.error, true);
      if (message.id) {
        const pending = state.pending.get(message.id);
        if (pending) {
          state.pending.delete(message.id);
          pending.reject(new Error(message.error));
        }
      }
      if (!message.id) failWorker(new Error(message.error));
      return;
    }
    const pending = state.pending.get(message.id);
    if (!pending) return;
    state.pending.delete(message.id);
    pending.resolve(JSON.parse(message.result));
  });
  setStatus("Loading parser…");
  state.worker.addEventListener("error", (event) => {
    failWorker(new Error(event.message || "Parser worker failed"));
  });
  state.worker.addEventListener("messageerror", () => {
    failWorker(new Error("Parser worker communication failed"));
  });
}

function parseFile(file) {
  const directKind = directPreviewKind(file);
  if (directKind) {
    return Promise.resolve({
      kind: directKind,
      name: displayName(file),
      size: formatFileSize(file.size),
      modified: file.lastModified ? formatModified(file.lastModified) : "",
    });
  }
  if (!state.worker) return Promise.reject(new Error("Parser is not ready"));
  if (state.parser.phase === "failed") return Promise.reject(state.parser.error);
  const id = ++state.requestID;
  const readSize = isArchive(file) ? file.size : Math.min(file.size, maxPreviewBytes);
  const worker = state.worker;
  return new Promise((resolve, reject) => {
    const pending = {resolve, reject};
    state.pending.set(id, pending);
    file.slice(0, readSize).arrayBuffer().then((data) => {
      if (state.pending.get(id) !== pending || state.worker !== worker || state.parser.phase === "failed") {
        if (state.pending.get(id) === pending) state.pending.delete(id);
        reject(state.parser.error || new Error("Parser is not ready"));
        return;
      }
      worker.postMessage({
        type: "preview",
        id,
        file: {name: file.name, type: file.type, size: file.size, lastModified: file.lastModified},
        data,
      }, [data]);
    }).catch((error) => {
      if (state.pending.get(id) === pending) state.pending.delete(id);
      reject(error);
    });
  });
}

async function selectFile(file) {
  state.current = file;
  renderFileList();
  preview.replaceChildren(message("Reading " + displayName(file) + "…"));
  try {
    const result = await parseFile(file);
    if (state.current !== file) return false;
    renderPreview(file, result);
    return true;
  } catch (error) {
    if (state.current !== file) return false;
    preview.replaceChildren(message(error.message, true));
    return false;
  }
}

function formatFileSize(size) {
  if (size < 1024) return `${size} B`;
  let value = size;
  const units = ["KB", "MB", "GB", "TB"];
  let unit = -1;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(1)} ${units[unit]}`;
}

function formatModified(timestamp) {
  const date = new Date(timestamp);
  const pad = (value) => String(value).padStart(2, "0");
  const offset = -date.getTimezoneOffset();
  const sign = offset >= 0 ? "+" : "-";
  const hours = Math.floor(Math.abs(offset) / 60);
  const minutes = Math.abs(offset) % 60;
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
    `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())} ` +
    `${sign}${pad(hours)}:${pad(minutes)}`;
}

function renderPreview(file, result) {
  if (state.objectURL) URL.revokeObjectURL(state.objectURL);
  state.objectURL = URL.createObjectURL(file);
  const header = document.createElement("header");
  header.className = "preview-header";
  const title = document.createElement("div");
  const heading = document.createElement("h2");
  heading.textContent = displayName(file);
  const meta = document.createElement("div");
  meta.className = "meta";
  meta.textContent = [result.size, result.modified && `Modified ${result.modified}`].filter(Boolean).join(" · ");
  title.append(heading, meta);
  const actions = document.createElement("div");
  actions.className = "actions";
  actions.append(downloadButton("Download original", state.objectURL, file.name));
  actions.append(copyButton(displayName(file)));
  header.append(title, actions);

  const body = document.createElement("div");
  body.className = "preview-body";
  if (result.error) body.append(message(result.error, true));
  else renderResult(body, file, result);
  preview.replaceChildren(header, body);
}

function renderResult(body, file, result) {
  if (result.kind === "image") {
    const image = document.createElement("img");
    image.className = "media-preview";
    image.src = state.objectURL;
    image.alt = file.name;
    body.append(image);
  } else if (result.kind === "audio" || result.kind === "video") {
    const media = document.createElement(result.kind);
    media.className = "media-player";
    media.controls = true;
    media.src = state.objectURL;
    body.append(media);
  } else if (result.kind === "pdf") {
    const frame = document.createElement("iframe");
    frame.className = "document-frame";
    frame.src = state.objectURL;
    frame.title = file.name;
    body.append(frame);
  } else if (result.kind === "html") {
    const frame = document.createElement("iframe");
    frame.className = "document-frame";
    frame.sandbox = "";
    frame.srcdoc = result.html;
    frame.title = file.name;
    body.append(frame);
  } else if (result.kind === "markdown" || result.kind === "org") {
    const article = document.createElement("article");
    article.className = "markdown-content";
    article.innerHTML = result.html;
    void renderMermaid(article);
    body.append(article);
  } else if (result.kind === "csv" || result.kind === "tsv") {
    body.append(table(result.rows || []));
  } else if (result.kind === "zip" || result.kind === "tar" || result.kind === "tar-gz") {
    body.append(archiveTable(result.entries || []));
  } else if (result.content !== undefined) {
    const pre = document.createElement("pre");
    pre.className = "code";
    pre.textContent = result.content;
    body.append(pre);
  } else {
    body.append(message("No preview available. Use Download original."));
  }

  function renderMermaid(article) {
    const nodes = [...article.querySelectorAll("pre.mermaid")];
    article.querySelectorAll("script").forEach((script) => script.remove());
    if (!nodes.length) return Promise.resolve();
    if (!state.mermaidPromise) {
      state.mermaidPromise = new Promise((resolve, reject) => {
        const script = document.createElement("script");
        script.src = "https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.min.js";
        script.onload = () => resolve(window.mermaid);
        script.onerror = () => reject(new Error("Unable to load Mermaid renderer."));
        document.head.append(script);
      });
    }
    return state.mermaidPromise
      .then((mermaid) => mermaid.run({nodes}))
      .catch((error) => setStatus(error.message, true));
  }
}

function table(rows) {
  const tableElement = document.createElement("table");
  for (const [rowIndex, row] of rows.entries()) {
    const tr = document.createElement("tr");
    for (const cell of row) {
      const td = document.createElement(rowIndex === 0 ? "th" : "td");
      td.textContent = cell;
      tr.append(td);
    }
    tableElement.append(tr);
  }
  return tableElement;
}

function archiveTable(entries) {
  const tableElement = document.createElement("table");
  tableElement.innerHTML = "<thead><tr><th>Name</th><th>Perms</th><th>Size</th><th>Modified</th></tr></thead>";
  const body = document.createElement("tbody");
  for (const entry of entries) {
    const row = document.createElement("tr");
    for (const value of [entry.name, entry.permissions, entry.uncompressedSize, entry.modified]) {
      const cell = document.createElement("td");
      cell.textContent = value;
      row.append(cell);
    }
    body.append(row);
  }
  tableElement.append(body);
  return tableElement;
}

function message(text, error = false) {
  const element = document.createElement("div");
  element.className = "empty-state large";
  if (error) element.classList.add("error");
  element.textContent = text;
  return element;
}

function downloadButton(label, href, name) {
  const link = document.createElement("a");
  link.className = "button";
  link.textContent = label;
  link.href = href;
  link.download = name;
  return link;
}

function copyButton(value) {
  const button = document.createElement("button");
  button.className = "button";
  button.type = "button";
  button.textContent = "Copy relative path";
  button.addEventListener("click", async () => {
    await navigator.clipboard.writeText(value);
    button.textContent = "Copied";
    setTimeout(() => { button.textContent = "Copy relative path"; }, 1200);
  });
  return button;
}

async function readDirectory(handle, prefix = "") {
  const files = [];
  for await (const [name, child] of handle.entries()) {
    if (child.kind === "file") {
      const file = await child.getFile();
      Object.defineProperty(file, "_peekdPath", {value: prefix + name});
      files.push(file);
    } else {
      files.push(...await readDirectory(child, prefix + name + "/"));
    }
  }
  return files;
}

async function restoreSources() {
  const token = beginSourceOperation("restoring");
  const isCurrent = () => sourceOperationCurrent(token);
  const stop = () => finishSourceOperation(token);
  let sources;
  try {
    sources = await listSources();
  } catch (error) {
    setStatus(`Local history unavailable: ${error.message}`, true);
    stop();
    return;
  }
  if (!isCurrent()) return;
  let restored = 0;
  let permissionNeeded = 0;
  for (const source of sources.sort((a, b) => b.updatedAt - a.updatedAt)) {
    if (!isCurrent()) return;
    try {
      if (source.kind === "url") {
        await loadURL(source.url, {persist: false, isCurrent});
        if (!isCurrent()) return;
        restored++;
        continue;
      }
      if (source.handle && typeof source.handle.queryPermission === "function" &&
          await source.handle.queryPermission({mode: "read"}) === "granted") {
        if (source.kind === "file") {
          const file = await source.handle.getFile();
          if (!isCurrent()) return;
          state.fileSources.set(file, source);
          addFiles([file]);
        } else if (source.kind === "directory") {
          const files = filterSourceFiles(await readDirectory(source.handle), source);
          if (!isCurrent()) return;
          for (const file of files) state.fileSources.set(file, source);
          addFiles(files);
        }
        restored++;
        continue;
      }
      if (source.file) {
        if (source.path && source.path !== source.file.name) {
          Object.defineProperty(source.file, "_peekdPath", {value: source.path});
        }
        state.fileSources.set(source.file, source);
        addFiles([source.file]);
        restored++;
        continue;
      }
      if (source.files) {
        for (const item of source.files) {
          if (item.path && item.path !== item.file.name) {
            Object.defineProperty(item.file, "_peekdPath", {value: item.path});
          }
        }
        const files = filterSourceFiles(source.files.map((item) => item.file), source);
        for (const file of files) state.fileSources.set(file, source);
        addFiles(files);
        restored++;
        continue;
      }

      if (source.handle) {
        permissionNeeded++;
      }
    } catch (error) {
      setStatus(`Unable to restore ${source.name || "source"}: ${error.message}`, true);
    }
  }
  if (permissionNeeded) {
    setStatus(`${permissionNeeded} saved source${permissionNeeded === 1 ? "" : "s"} need permission to reopen.`, true);
  } else if (restored) {
    setStatus("Restored recent files");
  }
  stop();
}

function filterSourceFiles(files, source) {
  const excluded = new Set(source.excludedPaths || []);
  return files.filter((file) => !excluded.has(file._peekdPath || file.name));
}

async function openFiles() {
  const token = beginSourceOperation("loading");
  if (!window.showOpenFilePicker) {
    fileInput.click();
    finishSourceOperation(token);
    return;
  }
  try {
    const handles = await window.showOpenFilePicker({multiple: true});
    const files = [];
    for (const handle of handles) {
      const file = await handle.getFile();
      const source = {key: `file:${crypto.randomUUID()}`, kind: "file", handle, file, path: file.name, name: file.name};
      state.fileSources.set(file, source);
      if (!sourceOperationCurrent(token)) return;
      await saveSourceBestEffort(source);
      if (!sourceOperationCurrent(token)) return;
      files.push(file);
    }
    addFiles(files, true);
  } finally {
    finishSourceOperation(token);
  }
}

async function openDirectory() {
  const token = beginSourceOperation("loading");
  if (window.showDirectoryPicker) {
    try {
      const handle = await window.showDirectoryPicker();
      const source = {key: `directory:${crypto.randomUUID()}`, kind: "directory", handle, name: handle.name};
      const files = await readDirectory(handle);
      if (!sourceOperationCurrent(token)) return;
      source.files = files.map((file) => ({path: file._peekdPath || file.name, file}));
      for (const file of files) state.fileSources.set(file, source);
      await saveSourceBestEffort(source);
      if (!sourceOperationCurrent(token)) return;
      addFiles(files, true);
    } finally {
      finishSourceOperation(token);
    }
    return;
  }
  directoryInput.click();
  finishSourceOperation(token);
}
$("open-file").addEventListener("click", () => {
  openFiles().catch((error) => { if (error.name !== "AbortError") setStatus(error.message, true); });
});
$("open-directory").addEventListener("click", () => {
  openDirectory().catch((error) => { if (error.name !== "AbortError") setStatus(error.message, true); });
});
async function saveSelectedFiles(files) {
  const token = beginSourceOperation("loading");
  addFiles(files, true);
  try {
    for (const file of files) {
      const source = {
        key: `snapshot:${crypto.randomUUID()}`,
        kind: "snapshot",
        file,
        path: file.webkitRelativePath || file.name,
        name: file.name,
      };
      state.fileSources.set(file, source);
      if (!sourceOperationCurrent(token)) return;
      await saveSourceBestEffort(source);
    }
  } finally {
    finishSourceOperation(token);
  }
}
fileInput.addEventListener("change", () => {
  saveSelectedFiles([...fileInput.files]).catch((error) => setStatus(error.message, true));
});
directoryInput.addEventListener("change", () => {
  saveSelectedFiles([...directoryInput.files]).catch((error) => setStatus(error.message, true));
});
$("url-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const url = urlInput.value.trim();
  if (!url) {
    setStatus("Enter a URL first.", true);
    urlInput.focus();
    return;
  }
  try {
    const token = beginSourceOperation("loading");
    try {
      await loadURL(url, {isCurrent: () => sourceOperationCurrent(token)});
    } finally {
      finishSourceOperation(token);
    }
  } catch (error) {
    setStatus(error.message, true);
  }
});
filter.addEventListener("input", renderFileList);
$("clear-files").addEventListener("click", () => {
  state.files.clear();
  state.current = null;
  if (state.objectURL) {
    URL.revokeObjectURL(state.objectURL);
    state.objectURL = null;
  }
  const token = beginSourceOperation("clearing");
  enqueueStorageTask(() => clearSavedSources())
    .catch((error) => setStatus(`Unable to clear saved sources: ${error.message}`, true))
    .finally(() => finishSourceOperation(token));
  renderFileList();
  preview.replaceChildren(message("Select a file to preview"));
});
document.addEventListener("dragover", (event) => event.preventDefault());
document.addEventListener("drop", (event) => {
  event.preventDefault();
  saveSelectedFiles([...event.dataTransfer.files]).catch((error) => setStatus(error.message, true));
});
document.addEventListener("keydown", (event) => {
  if (event.key === "/" && document.activeElement !== filter) { event.preventDefault(); filter.focus(); }
});

startWorker();
void restoreSources();
