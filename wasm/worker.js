/* global Go */
importScripts("wasm_exec.js");

const go = new Go();
let ready = false;

function handlePreview(event) {
  if (event.data.type !== "preview") return;
  if (!ready || typeof self.peekdPreview !== "function") {
    self.postMessage({
      type: "error",
      id: event.data.id,
      error: "WASM parser is still loading. Try again in a moment.",
    });
    return;
  }
  const file = event.data.file;
  try {
    const result = self.peekdPreview({
      name: file.name,
      type: file.type,
      size: file.size,
      lastModified: file.lastModified,
      data: new Uint8Array(event.data.data),
    });
    self.postMessage({type: "result", id: event.data.id, result});
  } catch (error) {
    self.postMessage({type: "error", id: event.data.id, error: String(error)});
  }
}

self.peekdReady = () => {
  ready = true;
};

WebAssembly.instantiateStreaming(fetch("peekd.wasm"), go.importObject)
  .then((result) => go.run(result.instance))
  .then(() => self.postMessage({type: "error", error: "WASM parser stopped"}))
  .catch((error) => {
    self.postMessage({type: "error", error: String(error)});
  });

self.onmessage = handlePreview;
