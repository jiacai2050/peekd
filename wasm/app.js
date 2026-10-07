const state = {
  files: new Map(),
  current: null,
  requestID: 0,
  worker: null,
  pending: new Map(),
  objectURL: null,
  mermaidPromise: null,
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

function setStatus(message, isError = false) {
  const status = $("status");
  status.textContent = message;
  status.classList.toggle("error", isError);
}

function displayName(file) {
  return file._peekdPath || file.webkitRelativePath || file.name;
}

function addFiles(files) {
  for (const file of files) state.files.set(displayName(file), file);
  renderFileList();
  if (!state.current && state.files.size) selectFile([...state.files.values()][0]);
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

async function loadURL(url) {
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
  const file = new File([blob], remoteFileName(parsed.href, response.headers.get("content-disposition")), {
    type: response.headers.get("content-type")?.split(";")[0] || blob.type,
    lastModified: Date.now(),
  });
  addFiles([file]);
  selectFile(file);
  setStatus("Ready");
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
    const button = document.createElement("button");
    button.className = "file-row";
    button.classList.toggle("selected", state.current === file);
    button.type = "button";
    button.title = name;
    const icon = document.createElement("span");
    icon.className = "file-icon";
    icon.textContent = iconFor(file.name);
    const label = document.createElement("span");
    label.className = "file-name";
    label.textContent = name;
    button.append(icon, label);
    button.addEventListener("click", () => selectFile(file));
    fileList.append(button);
  }
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
  state.worker.addEventListener("message", (event) => {
    const message = event.data;
    if (message.type === "ready") {
      setStatus("Ready");
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
      return;
    }
    const pending = state.pending.get(message.id);
    if (!pending) return;
    state.pending.delete(message.id);
    pending.resolve(JSON.parse(message.result));
  });
  setStatus("Loading parser…");
  state.worker.addEventListener("error", (event) => setStatus(event.message || "Parser worker failed", true));
}

function parseFile(file) {
  if (!state.worker) return Promise.reject(new Error("Parser is not ready"));
  const directKind = directPreviewKind(file);
  if (directKind) {
    return Promise.resolve({
      kind: directKind,
      name: displayName(file),
      size: formatFileSize(file.size),
      modified: file.lastModified ? new Date(file.lastModified).toISOString().slice(0, 19).replace("T", " ") : "",
    });
  }
  const id = ++state.requestID;
  const readSize = isArchive(file) ? file.size : Math.min(file.size, maxPreviewBytes);
  return file.slice(0, readSize).arrayBuffer().then((data) => new Promise((resolve, reject) => {
    state.pending.set(id, {resolve, reject});
    state.worker.postMessage({
      type: "preview",
      id,
      file: {name: file.name, type: file.type, size: file.size, lastModified: file.lastModified},
      data,
    }, [data]);
  }));
}

async function selectFile(file) {
  state.current = file;
  renderFileList();
  preview.replaceChildren(message("Reading " + displayName(file) + "…"));
  try {
    const result = await parseFile(file);
    if (state.current !== file) return;
    renderPreview(file, result);
  } catch (error) {
    if (state.current !== file) return;
    preview.replaceChildren(message(error.message, true));
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

$("open-file").addEventListener("click", () => fileInput.click());
$("open-directory").addEventListener("click", async () => {
  if (window.showDirectoryPicker) {
    try { addFiles(await readDirectory(await window.showDirectoryPicker())); }
    catch (error) { if (error.name !== "AbortError") setStatus(error.message, true); }
  } else directoryInput.click();
});
fileInput.addEventListener("change", () => addFiles(fileInput.files));
directoryInput.addEventListener("change", () => addFiles(directoryInput.files));
$("url-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const url = urlInput.value.trim();
  if (!url) {
    setStatus("Enter a URL first.", true);
    urlInput.focus();
    return;
  }
  try {
    await loadURL(url);
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
  renderFileList();
  preview.replaceChildren(message("Select a file to preview"));
});
document.addEventListener("dragover", (event) => event.preventDefault());
document.addEventListener("drop", (event) => {
  event.preventDefault();
  addFiles(event.dataTransfer.files);
});
document.addEventListener("keydown", (event) => {
  if (event.key === "/" && document.activeElement !== filter) { event.preventDefault(); filter.focus(); }
});

startWorker();
