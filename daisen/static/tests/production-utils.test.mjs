import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import test from "node:test";

import ts from "typescript";

const toFileUrl = (path) => pathToFileURL(path).href;

const escapeRegExp = (value) => value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

const replaceModuleSpecifier = (source, specifier, replacementUrl) =>
  source.replace(new RegExp(`(["'])${escapeRegExp(specifier)}\\1`, "g"), JSON.stringify(replacementUrl));

const importProductionModule = async (sourcePath, tempDir, dependencyUrls) => {
  const source = await readFile(new URL(sourcePath, import.meta.url), "utf8");
  const transpiled = ts.transpileModule(source, {
    compilerOptions: {
      allowSyntheticDefaultImports: true,
      esModuleInterop: true,
      module: ts.ModuleKind.ES2022,
      moduleResolution: ts.ModuleResolutionKind.Bundler,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;

  let runnableSource = transpiled.replace(
    /import\s+["']katex\/dist\/katex\.min\.css["'];?\s*/g,
    "",
  );

  for (const [specifier, replacementUrl] of Object.entries(dependencyUrls)) {
    runnableSource = replaceModuleSpecifier(runnableSource, specifier, replacementUrl);
  }

  const modulePath = join(tempDir, `${sourcePath.split("/").at(-1)}.mjs`);
  await writeFile(modulePath, runnableSource);
  return import(`${toFileUrl(modulePath)}?cacheBust=${Date.now()}-${Math.random()}`);
};

const writeTempModule = async (tempDir, fileName, source) => {
  const modulePath = join(tempDir, fileName);
  await writeFile(modulePath, source);
  return toFileUrl(modulePath);
};

class FakeClassList {
  #classes = new Set();

  constructor(owner) {
    this.owner = owner;
  }

  add(...classes) {
    classes.forEach((className) => this.#classes.add(className));
    this.#syncOwner();
  }

  remove(...classes) {
    classes.forEach((className) => this.#classes.delete(className));
    this.#syncOwner();
  }

  contains(className) {
    return this.#classes.has(className);
  }

  setFromString(value) {
    this.#classes = new Set(String(value).split(/\s+/).filter(Boolean));
    this.#syncOwner();
  }

  #syncOwner() {
    this.owner._className = [...this.#classes].join(" ");
  }
}

class FakeElement {
  constructor(tagName, ownerDocument) {
    this.tagName = tagName.toUpperCase();
    this.ownerDocument = ownerDocument;
    this.style = {};
    this.dataset = {};
    this.children = [];
    this.parentNode = null;
    this.eventListeners = new Map();
    this.attributes = new Map();
    this.classList = new FakeClassList(this);
    this._className = "";
    this._id = "";
    this._innerHTML = "";
    this._textContent = "";
    this._syntheticMathElements = [];
    this.files = null;
    this.value = "";
    this.disabled = false;
    this.selected = false;
    this.checked = false;
    this.scrollHeight = 38;
    this.scrollTop = 0;
    this.offsetWidth = 600;
    this.offsetHeight = 400;
  }

  set id(value) {
    this._id = String(value);
  }

  get id() {
    return this._id;
  }

  set className(value) {
    this.classList.setFromString(value);
  }

  get className() {
    return this._className;
  }

  set innerHTML(value) {
    this._innerHTML = String(value);
    if (this._innerHTML === "") {
      this.children = [];
    }
    this._syntheticMathElements = [...this._innerHTML.matchAll(
      /<span\s+class=["']math["']\s+data-display=["']([^"']+)["']>([\s\S]*?)<\/span>/g,
    )].map((match) => new FakeMathElement(match[2], match[1]));
  }

  get innerHTML() {
    return this._innerHTML;
  }

  set textContent(value) {
    this._textContent = String(value);
    this._innerHTML = this._textContent;
  }

  get textContent() {
    return this._textContent || this._innerHTML.replace(/<[^>]*>/g, "");
  }

  appendChild(child) {
    child.parentNode = this;
    this.children.push(child);
    return child;
  }

  remove() {
    if (!this.parentNode) return;
    this.parentNode.children = this.parentNode.children.filter((child) => child !== this);
    this.parentNode = null;
  }

  addEventListener(type, listener) {
    const listeners = this.eventListeners.get(type) ?? [];
    listeners.push(listener);
    this.eventListeners.set(type, listeners);
  }

  dispatchEvent(event) {
    event.target ??= this;
    event.currentTarget = this;
    for (const listener of this.eventListeners.get(event.type) ?? []) {
      listener.call(this, event);
    }
    return !event.defaultPrevented;
  }

  click() {
    this.onclick?.({ target: this, preventDefault() {} });
  }

  setAttribute(name, value) {
    this.attributes.set(name, String(value));
    if (name === "id") this.id = value;
    if (name === "class") this.className = value;
  }

  getAttribute(name) {
    if (name === "id") return this.id;
    if (name === "class") return this.className;
    return this.attributes.get(name) ?? null;
  }

  getBoundingClientRect() {
    return { height: this.offsetHeight, left: 0, top: 0, width: this.offsetWidth };
  }

  focus() {}

  querySelector(selector) {
    return this.querySelectorAll(selector)[0] ?? null;
  }

  querySelectorAll(selector) {
    const matches = [];

    if (selector === ".math") {
      matches.push(...this._syntheticMathElements);
    }

    const visit = (element) => {
      if (matchesSelector(element, selector)) {
        matches.push(element);
      }
      if (selector === ".math") {
        matches.push(...element._syntheticMathElements);
      }
      element.children.forEach(visit);
    };

    this.children.forEach(visit);
    return matches;
  }
}

class FakeMathElement {
  constructor(textContent, displayMode) {
    this.textContent = textContent;
    this.innerHTML = textContent;
    this.displayMode = displayMode;
  }

  getAttribute(name) {
    return name === "data-display" ? this.displayMode : null;
  }
}

class FakeDocument {
  constructor() {
    this.head = new FakeElement("head", this);
    this.body = new FakeElement("body", this);
    this.eventListeners = new Map();
  }

  createElement(tagName) {
    return new FakeElement(tagName, this);
  }

  addEventListener(type, listener) {
    const listeners = this.eventListeners.get(type) ?? [];
    listeners.push(listener);
    this.eventListeners.set(type, listeners);
  }

  removeEventListener(type, listener) {
    const listeners = this.eventListeners.get(type) ?? [];
    this.eventListeners.set(type, listeners.filter((candidate) => candidate !== listener));
  }

  dispatchEvent(event) {
    for (const listener of this.eventListeners.get(event.type) ?? []) {
      listener.call(this, event);
    }
  }

  getElementById(id) {
    return [this.head, this.body].flatMap((root) => allDescendants(root, true)).find((el) => el.id === id) ?? null;
  }

  querySelector(selector) {
    return this.body.querySelector(selector) ?? this.head.querySelector(selector);
  }

  querySelectorAll(selector) {
    return [...this.head.querySelectorAll(selector), ...this.body.querySelectorAll(selector)];
  }
}

class FakeFileReader {
  readAsText(file) {
    this.onload?.({ target: { result: `text:${file.name}` } });
  }

  readAsDataURL(file) {
    this.onload?.({ target: { result: `data:${file.name}` } });
  }
}

const matchesSelector = (element, selector) => {
  if (selector.startsWith("#")) return element.id === selector.slice(1);
  if (selector.startsWith(".")) return element.classList.contains(selector.slice(1));
  return element.tagName.toLowerCase() === selector.toLowerCase();
};

const allDescendants = (root, includeRoot = false) => {
  const elements = includeRoot ? [root] : [];
  for (const child of root.children) {
    elements.push(child, ...allDescendants(child));
  }
  return elements;
};

const installBrowserHarness = () => {
  const previous = {
    FileReader: globalThis.FileReader,
    alert: globalThis.alert,
    clearTimeout: globalThis.clearTimeout,
    consoleLog: console.log,
    document: globalThis.document,
    fetch: globalThis.fetch,
    setTimeout: globalThis.setTimeout,
    window: globalThis.window,
  };

  const alerts = [];
  const document = new FakeDocument();
  const window = {
    addEventListener() {},
    alert: (message) => alerts.push(message),
    getComputedStyle: (element) => ({ width: element.style.width ?? "600px" }),
    history: { pushState() {}, replaceState() {} },
    location: { href: "http://localhost/task", pathname: "/", search: "" },
  };

  globalThis.document = document;
  globalThis.window = window;
  globalThis.alert = window.alert;
  globalThis.FileReader = FakeFileReader;
  globalThis.setTimeout = (callback) => {
    callback();
    return 0;
  };
  globalThis.clearTimeout = () => {};
  console.log = () => {};

  return {
    alerts,
    document,
    restore() {
      globalThis.document = previous.document;
      globalThis.window = previous.window;
      globalThis.alert = previous.alert;
      globalThis.FileReader = previous.FileReader;
      globalThis.setTimeout = previous.setTimeout;
      globalThis.clearTimeout = previous.clearTimeout;
      console.log = previous.consoleLog;
      globalThis.fetch = previous.fetch;
    },
  };
};

const html2CanvasStub = `
  export default async function html2canvas() {
    return {
      height: 1,
      width: 1,
      getContext: () => ({ drawImage() {} }),
      toBlob: (callback) => callback({ size: 1 }),
      toDataURL: () => "data:image/png;base64,stub",
    };
  }
`;

const chatpanelRequestsStub = `
  export const GPTRequest = undefined;
  export const UnitContent = undefined;
  export const sendGetGitHubIsAvailable = async () => ({ available: 0, routine_keys: [] });
  export const sendPostGPT = async () => ({ content: "stub response", totalTokens: -1 });
`;

test("chat panel renders assistant markdown and math through the shared helper module", async () => {
  const tempDir = await mkdtemp(join(tmpdir(), "daisen-production-utils-"));
  const browser = installBrowserHarness();

  try {
    const chatMarkdownUrl = await writeTempModule(tempDir, "chat-markdown-stub.mjs", `
      export const markdownInputs = [];
      export const mathRoots = [];
      export function renderChatMarkdown(text) {
        markdownInputs.push(text);
        return '<span class="rendered-from-helper">helper rendered: ' + text + '</span>';
      }
      export function renderMathInElement(root) {
        mathRoots.push(root);
        root.dataset.mathRenderedByHelper = "true";
      }
    `);
    const uploadValidationUrl = await writeTempModule(tempDir, "upload-validation-stub.mjs", `
      export const FILE_UPLOAD_ACCEPT = "shared-file-accept";
      export const IMAGE_UPLOAD_ACCEPT = "shared-image-accept";
      export const isImageUploadCandidate = () => false;
      export const validateUploadedFile = () => ({ valid: true });
    `);
    const html2CanvasUrl = await writeTempModule(tempDir, "html2canvas-stub.mjs", html2CanvasStub);
    const requestsUrl = await writeTempModule(tempDir, "chatpanelrequests-stub.mjs", chatpanelRequestsStub);

    const [{ ChatPanel }, chatMarkdown] = await Promise.all([
      importProductionModule("../src/chatpanel.ts", tempDir, {
        "./chatpanelrequests": requestsUrl,
        "./utils/chatMarkdown.mjs": chatMarkdownUrl,
        "./utils/uploadValidation.mjs": uploadValidationUrl,
        html2canvas: html2CanvasUrl,
      }),
      import(chatMarkdownUrl),
    ]);

    const panel = new ChatPanel();
    panel._chatMessages = [
      { role: "assistant", content: [{ type: "text", text: "**bold** and \\(x+1\\)" }] },
    ];

    panel._showChatPanel();

    assert.deepEqual(chatMarkdown.markdownInputs, ["**bold** and \\(x+1\\)"]);
    assert.equal(chatMarkdown.mathRoots.length, 1);
    assert.equal(chatMarkdown.mathRoots[0].dataset.mathRenderedByHelper, "true");

    const renderedBotMessage = allDescendants(browser.document.body, true).find((element) =>
      element.innerHTML.includes("rendered-from-helper"),
    );
    assert.ok(renderedBotMessage, "assistant message should contain the helper-rendered HTML");
    assert.match(renderedBotMessage.innerHTML, /<b>Daisen Bot:<\/b>/);
  } finally {
    browser.restore();
    await rm(tempDir, { force: true, recursive: true });
  }
});

test("chat panel validates inputs and dispatches dropped images through shared upload helpers", async () => {
  const tempDir = await mkdtemp(join(tmpdir(), "daisen-production-utils-"));
  const browser = installBrowserHarness();

  try {
    const chatMarkdownUrl = await writeTempModule(tempDir, "chat-markdown-stub.mjs", `
      export const renderChatMarkdown = (text) => text;
      export const renderMathInElement = () => {};
    `);
    const uploadValidationUrl = await writeTempModule(tempDir, "upload-validation-stub.mjs", `
      export const FILE_UPLOAD_ACCEPT = "shared-file-accept";
      export const IMAGE_UPLOAD_ACCEPT = "shared-image-accept";
      export const candidateCalls = [];
      export const validationCalls = [];
      export const isImageUploadCandidate = (file) => {
        candidateCalls.push(file.name);
        return file.name.includes("image-by-helper");
      };
      export const validateUploadedFile = (file, type) => {
        validationCalls.push({ name: file.name, type });
        if (file.name.startsWith("bad")) {
          return { valid: false, error: "blocked " + type + ": " + file.name };
        }
        return { valid: true };
      };
    `);
    const html2CanvasUrl = await writeTempModule(tempDir, "html2canvas-stub.mjs", html2CanvasStub);
    const requestsUrl = await writeTempModule(tempDir, "chatpanelrequests-stub.mjs", chatpanelRequestsStub);

    const [{ ChatPanel }, uploadValidation] = await Promise.all([
      importProductionModule("../src/chatpanel.ts", tempDir, {
        "./chatpanelrequests": requestsUrl,
        "./utils/chatMarkdown.mjs": chatMarkdownUrl,
        "./utils/uploadValidation.mjs": uploadValidationUrl,
        html2canvas: html2CanvasUrl,
      }),
      import(uploadValidationUrl),
    ]);

    const panel = new ChatPanel();
    panel._showChatPanel();

    const fileInputs = browser.document
      .querySelectorAll("input")
      .filter((input) => input.type === "file");
    assert.equal(fileInputs.length, 2);

    const regularFileInput = fileInputs.find((input) => input.accept === "shared-file-accept");
    const imageFileInput = fileInputs.find((input) => input.accept === "shared-image-accept");
    assert.ok(regularFileInput, "regular file input should use the shared accept string");
    assert.ok(imageFileInput, "image input should use the shared accept string");

    regularFileInput.files = [{ name: "trace.csv", size: 2048, type: "text/csv" }];
    regularFileInput.onchange();
    assert.deepEqual(uploadValidation.validationCalls.at(-1), { name: "trace.csv", type: "file" });
    assert.deepEqual(panel._uploadedFiles.at(-1), {
      content: "text:trace.csv",
      id: 1,
      name: "trace.csv",
      size: "2.0 KB",
      type: "file",
    });

    imageFileInput.files = [{ name: "bad-image.png", size: 512, type: "image/png" }];
    imageFileInput.onchange();
    assert.deepEqual(uploadValidation.validationCalls.at(-1), { name: "bad-image.png", type: "image" });
    assert.deepEqual(browser.alerts, ["blocked image: bad-image.png"]);
    assert.equal(panel._uploadedFiles.length, 1, "rejected uploads should not be added");

    const droppedFile = { name: "diagram-image-by-helper.bin", size: 1024, type: "application/octet-stream" };
    const chatPanelElement = browser.document.getElementById("chat-panel");
    let prevented = false;
    chatPanelElement.dispatchEvent({
      type: "drop",
      dataTransfer: { files: [droppedFile] },
      preventDefault() {
        prevented = true;
        this.defaultPrevented = true;
      },
    });

    assert.equal(prevented, true);
    assert.deepEqual(uploadValidation.candidateCalls, ["diagram-image-by-helper.bin"]);
    assert.deepEqual(uploadValidation.validationCalls.at(-1), {
      name: "diagram-image-by-helper.bin",
      type: "image",
    });
    assert.equal(panel._uploadedFiles.at(-1).type, "image");
    assert.equal(panel._uploadedFiles.at(-1).content, "data:diagram-image-by-helper.bin");
  } finally {
    browser.restore();
    await rm(tempDir, { force: true, recursive: true });
  }
});

test("app refreshes mode by passing the production response text to the shared parser", async () => {
  const tempDir = await mkdtemp(join(tmpdir(), "daisen-production-utils-"));
  const previousFetch = globalThis.fetch;

  try {
    const modeUrl = await writeTempModule(tempDir, "mode-stub.mjs", `
      export const payloads = [];
      export function parseModeResponse(payload) {
        payloads.push(payload);
        return "replay";
      }
    `);
    const dashboardPageUrl = await writeTempModule(tempDir, "dashboardpage-stub.mjs", `
      export default class DashboardPage {
        layout() {}
        render() {}
      }
    `);
    const taskPageUrl = await writeTempModule(tempDir, "taskpage-stub.mjs", `
      export class TaskPage {
        layout() {}
        setTimeRange() {}
        showComponent() {}
        showTask() {}
      }
    `);
    const mouseEventHandlerUrl = await writeTempModule(tempDir, "mouseeventhandler-stub.mjs", `
      export class MouseEventHandler {
        register() {}
      }
    `);

    const payload = '{"mode":"live","ignored":"the parser decides"}';
    const fetchedUrls = [];
    globalThis.fetch = async (url) => {
      fetchedUrls.push(url);
      return { text: async () => payload };
    };

    const [{ default: App }, mode] = await Promise.all([
      importProductionModule("../src/app.ts", tempDir, {
        "./dashboardpage": dashboardPageUrl,
        "./mouseeventhandler": mouseEventHandlerUrl,
        "./taskpage": taskPageUrl,
        "./utils/mode.mjs": modeUrl,
      }),
      import(modeUrl),
    ]);

    const app = new App();
    await app._refreshMode();

    assert.deepEqual(fetchedUrls, ["/api/mode"]);
    assert.deepEqual(mode.payloads, [payload]);
    assert.equal(app._mode, "replay");
  } finally {
    globalThis.fetch = previousFetch;
    await rm(tempDir, { force: true, recursive: true });
  }
});
