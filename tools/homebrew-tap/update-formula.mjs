// SPDX-License-Identifier: AGPL-3.0-or-later
// Prepare a reviewed Homebrew update from the latest published CLI release.
import { createHash } from "node:crypto";
import { appendFile, readFile, writeFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";

const SOURCE = "giacomobartoloni/awl";
const TAP = "giacomobartoloni/homebrew-tap";
const BRANCH = "automation/update-awl";
const API = "https://api.github.com/repos/";
const TAG = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const VERSION = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const HASH = /^[a-f0-9]{64}$/;
function assert(condition, reason) {
  if (!condition) throw new Error(reason);
}

export function compareVersions(a, b) {
  assert(VERSION.test(a) && VERSION.test(b), "Invalid canonical version");
  const left = a.split(".").map(BigInt);
  const right = b.split(".").map(BigInt);
  for (let i = 0; i < 3; i += 1) {
    if (left[i] > right[i]) return 1;
    if (left[i] < right[i]) return -1;
  }
  return 0;
}

export function latestPublishedRelease(releases) {
  assert(Array.isArray(releases), "GitHub releases response is not an array");
  const eligible = releases.filter((item) =>
    item && !item.draft && !item.prerelease && item.published_at && TAG.test(item.tag_name)
  );
  assert(eligible.length > 0, "No eligible published canonical releases");
  const seen = new Set();
  for (const item of eligible) {
    assert(!seen.has(item.tag_name), "Duplicate public release tag: " + item.tag_name);
    seen.add(item.tag_name);
  }
  eligible.sort((a, b) =>
    compareVersions(b.tag_name.slice(1), a.tag_name.slice(1))
  );
  return eligible[0];
}


const TARGETS = ["darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64"];
const PAIR = /^([ \t]*url "https:\/\/github\.com\/giacomobartoloni\/awl\/releases\/download\/v)([0-9]+\.[0-9]+\.[0-9]+)(\/awl_)([0-9]+\.[0-9]+\.[0-9]+)(_((?:darwin|linux)_(?:arm64|amd64))\.tar\.gz"[ \t]*\r?\n[ \t]*sha256 ")([a-f0-9]{64})("[ \t]*)$/gm;
const README_VERSION = /^(The formula installs the official \x60v)([0-9]+\.[0-9]+\.[0-9]+)(\x60 prebuilt CLI archive with a pinned SHA-256 digest\.)/gm;

export function parseFormula(content) {
  assert(content.includes("class Awl < Formula"), "Unexpected formula class");
  const pairs = [...content.matchAll(PAIR)];
  assert(pairs.length === 4 && [...content.matchAll(/^\s*url\s+/gm)].length === 4 &&
    [...content.matchAll(/^\s*sha256\s+/gm)].length === 4, "Expected exactly four URL/checksum pairs");
  const version = pairs[0][2];
  const hashes = {};
  pairs.forEach((match, index) => {
    assert(match[2] === version && match[4] === version && VERSION.test(version) &&
      match[6] === TARGETS[index], "Inconsistent formula version or target");
    hashes[match[6]] = match[7];
  });
  // Verify the exact OS/architecture nesting as well as the URL order.
  const layout = content.replace(PAIR, (_m, _a, _b, _c, _d, _e, target) => "      " + target);
  const expected = [
    "  on_macos do", "    on_arm do", "      darwin_arm64", "    end", "",
    "    on_intel do", "      darwin_amd64", "    end", "  end", "",
    "  on_linux do", "    on_arm do", "      linux_arm64", "    end", "",
    "    on_intel do", "      linux_amd64", "    end", "  end"
  ].join("\n");
  assert(layout.replace(/\r\n/g, "\n").includes(expected), "Unexpected formula platform layout");
  return { version, hashes };
}

export function updateFormula(content, version, hashes) {
  const pinned = parseFormula(content);
  assert(VERSION.test(version) && compareVersions(version, pinned.version) >= 0,
    "Refusing invalid version or downgrade");
  for (const target of TARGETS) assert(HASH.test(hashes[target]), "Invalid hash: " + target);
  const updated = content.replace(PAIR, (_m, a, _b, c, _d, e, target, _hash, suffix) =>
    a + version + c + version + e + hashes[target] + suffix);
  const result = parseFormula(updated);
  assert(result.version === version && TARGETS.every(t => result.hashes[t] === hashes[t]),
    "Rendered formula failed verification");
  return updated;
}

export function updateReadme(content, oldVersion, newVersion) {
  const matches = [...content.matchAll(README_VERSION)];
  assert(matches.length === 1 && matches[0][2] === oldVersion, "Unexpected README version");
  return content.replace(README_VERSION, (_m, prefix, _version, suffix) => prefix + newVersion + suffix);
}

export function parseChecksums(content) {
  const entries = new Map();
  for (const line of content.trimEnd().split(/\r?\n/)) {
    const match = /^([a-f0-9]{64}) [ *](\S+)$/.exec(line);
    assert(match, "Malformed checksums.txt entry");
    assert(!entries.has(match[2]), "Duplicate checksum entry: " + match[2]);
    entries.set(match[2], match[1]);
  }
  return entries;
}

export function expectedAssets(release) {
  assert(release && !release.draft && !release.prerelease && release.published_at &&
    TAG.test(release.tag_name), "Not a stable published release");
  const version = release.tag_name.slice(1);
  const names = Object.fromEntries(TARGETS.map(t => [t, "awl_" + version + "_" + t + ".tar.gz"]));
  names.manifest = "checksums.txt";
  assert(Array.isArray(release.assets), "Missing release assets");
  const selected = {};
  for (const [key, name] of Object.entries(names)) {
    const found = release.assets.filter(a => a.name === name);
    assert(found.length === 1, "Missing or duplicate release asset: " + name);
    const asset = found[0];
    assert(asset.browser_download_url === "https://github.com/" + SOURCE + "/releases/download/" +
      release.tag_name + "/" + name && Number.isSafeInteger(asset.size) && asset.size > 0 &&
      (!asset.state || asset.state === "uploaded"), "Invalid asset metadata: " + name);
    selected[key] = asset;
  }
  return selected;
}

async function getJson(path, token, fetcher = fetch) {
  const url = API + path;
  const response = await fetcher(url, {
    headers: {
      Accept: "application/vnd.github+json",
      "User-Agent": "awl-homebrew-tap-updater",
      ...(token ? { Authorization: "Bearer " + token } : {})
    },
    signal: AbortSignal.timeout(30000)
  });
  assert(response.ok, "GitHub API request failed: " + response.status + " " + path);
  return response.json();
}

export async function listPublicReleases(token, fetcher = fetch) {
  const releases = [];
  for (let page = 1; page <= 20; page += 1) {
    const items = await getJson(SOURCE + "/releases?per_page=100&page=" + page,
      token, fetcher);
    assert(Array.isArray(items) && items.length <= 100, "Invalid release page");
    releases.push(...items);
    if (items.length < 100) return releases;
  }
  throw new Error("Release catalog exceeds safe pagination bound");
}

async function download(url, maxBytes, fetcher = fetch) {
  const response = await fetcher(url, {
    redirect: "follow",
    headers: { "User-Agent": "awl-homebrew-tap-updater" },
    signal: AbortSignal.timeout(120000)
  });
  assert(response.ok && response.body, "Asset download failed: " + response.status);
  let size = 0;
  const chunks = [];
  const digest = createHash("sha256");
  for await (const chunk of response.body) {
    size += chunk.byteLength;
    assert(size <= maxBytes, "Published asset exceeds size limit");
    digest.update(chunk);
    if (maxBytes <= 1024 * 1024) chunks.push(Buffer.from(chunk));
  }
  assert(size > 0, "Published asset is empty");
  return { size, digest: digest.digest("hex"), text: Buffer.concat(chunks).toString("utf8") };
}


export async function verifiedReleaseHashes(release, fetcher = fetch) {
  const assets = expectedAssets(release);
  const manifest = await download(assets.manifest.browser_download_url, 1024 * 1024, fetcher);
  assert(manifest.size === assets.manifest.size, "Manifest size mismatch");
  if (assets.manifest.digest) assert(assets.manifest.digest === "sha256:" + manifest.digest,
    "Manifest digest differs from release metadata");
  const sums = parseChecksums(manifest.text);
  const hashes = {};
  for (const target of TARGETS) {
    const asset = assets[target];
    const actual = await download(asset.browser_download_url, 128 * 1024 * 1024, fetcher);
    assert(actual.size === asset.size && actual.digest === sums.get(asset.name),
      "Archive size or checksum mismatch: " + asset.name);
    if (asset.digest) assert(asset.digest === "sha256:" + actual.digest,
      "Archive digest differs from release metadata: " + asset.name);
    hashes[target] = actual.digest;
  }
  return hashes;
}

export async function assertAutomationBranchOwnership(token, appSlug, fetcher = fetch) {
  assert(token && appSlug && /^[a-z0-9-]+$/i.test(appSlug),
    "Missing GitHub App token or app slug");
  const refPath = TAP + "/git/ref/heads/" + BRANCH;
  const refResponse = await fetcher(API + refPath, {
    headers: { Accept: "application/vnd.github+json",
      Authorization: "Bearer " + token,
      "User-Agent": "awl-homebrew-tap-updater" },
    signal: AbortSignal.timeout(30000)
  });
  if (refResponse.status === 404) return;
  assert(refResponse.ok, "Cannot verify existing automation branch");
  const query = TAP + "/pulls?state=open&head=" +
    encodeURIComponent("giacomobartoloni:" + BRANCH) + "&per_page=100";
  const pulls = await getJson(query, token, fetcher);
  assert(Array.isArray(pulls) && pulls.length === 1,
    "Automation branch exists without one open owned PR");
  const pr = pulls[0];
  assert(pr.user?.login?.toLowerCase() === (appSlug + "[bot]").toLowerCase() &&
    pr.head?.ref === BRANCH && pr.head?.repo?.full_name === TAP &&
    pr.base?.ref === "main" && pr.body?.includes("<!-- awl Tap Automation -->"),
  "Existing automation PR has unexpected owner, branch, or base");
  const files = await getJson(TAP + "/pulls/" + pr.number + "/files?per_page=100",
    token, fetcher);
  assert(Array.isArray(files) && files.length === 2 &&
    files.map(f => f.filename).sort().join(",") === "Formula/awl.rb,README.md",
  "Existing automation PR contains unexpected files");
  const file = await getJson(TAP + "/contents/Formula/awl.rb?ref=" + encodeURIComponent(BRANCH), token, fetcher);
  assert(file.encoding === "base64" && typeof file.content === "string", "Cannot read existing formula");
  const previous = parseFormula(Buffer.from(file.content, "base64").toString("utf8"));
  assert(VERSION.test(process.env.AWL_TARGET_VERSION || "") && compareVersions(process.env.AWL_TARGET_VERSION, previous.version) >= 0,
    "Refusing to downgrade existing automation PR");
}

async function emitOutputs(values) {
  if (process.env.GITHUB_OUTPUT) {
    await appendFile(process.env.GITHUB_OUTPUT,
      Object.entries(values).map(([key, value]) => key + "=" + value).join("\n") + "\n");
  }
}


async function main() {
  if (process.argv[2] === "--guard") {
    await assertAutomationBranchOwnership(process.env.TAP_GITHUB_TOKEN, process.env.TAP_APP_SLUG);
    console.log("Automation PR ownership check passed");
    return;
  }
  const [formulaPath, readmePath] = process.argv.slice(2);
  assert(formulaPath && readmePath, "Expected formula and README paths");
  if (process.env.GITHUB_EVENT_NAME === "push" && !TAG.test(process.env.GITHUB_REF_NAME || "")) {
    await emitOutputs({ changed: "false" });
    console.log("Noncanonical or prerelease tag; no tap update");
    return;
  }
  const oldText = await readFile(formulaPath, "utf8");
  const oldReadme = await readFile(readmePath, "utf8");
  const pinned = parseFormula(oldText);
  const release = latestPublishedRelease(await listPublicReleases(process.env.GITHUB_TOKEN));
  const version = release.tag_name.slice(1);
  const releaseUrl = "https://github.com/" + SOURCE + "/releases/tag/" + release.tag_name;
  if (compareVersions(version, pinned.version) < 0) {
    await emitOutputs({ changed: "false", version });
    console.log("Tap pins a newer version; no downgrade");
    return;
  }
  const hashes = await verifiedReleaseHashes(release);
  const updated = updateFormula(oldText, version, hashes);
  const readme = updateReadme(oldReadme, pinned.version, version);
  if (version === pinned.version) {
    assert(updated === oldText, "Published checksums differ from already-pinned version; review manually");
    await emitOutputs({ changed: "false", version });
    console.log("Tap already pins the verified release; no PR");
    return;
  }
  await writeFile(formulaPath, updated, "utf8");
  await writeFile(readmePath, readme, "utf8");
  await emitOutputs({ changed: "true", version, release_url: releaseUrl });
  console.log("Prepared formula update: " + pinned.version + " -> " + version);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch(error => {
    console.error("Homebrew tap update failed: " + error.message);
    process.exitCode = 1;
  });
}
