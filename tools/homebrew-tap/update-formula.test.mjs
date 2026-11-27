import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { compareVersions, latestPublishedRelease, parseFormula, updateFormula,
  updateReadme, parseChecksums, expectedAssets, verifiedReleaseHashes,
  assertAutomationBranchOwnership } from "./update-formula.mjs";

const fixture = "# SPDX-License-Identifier: AGPL-3.0-only\n\nclass Awl < Formula\n  desc \"Local-first CLI for Markdown-based coding-agent worklogs\"\n  homepage \"https://github.com/giacomobartoloni/awl\"\n  license \"AGPL-3.0-only\"\n\n  depends_on \"git\"\n\n  on_macos do\n    on_arm do\n      url \"https://github.com/giacomobartoloni/awl/releases/download/v0.3.0/awl_0.3.0_darwin_arm64.tar.gz\"\n      sha256 \"bd3665fe180a3996e31207cef61ef5d9b2e35160a148b38da511c241be26528f\"\n    end\n\n    on_intel do\n      url \"https://github.com/giacomobartoloni/awl/releases/download/v0.3.0/awl_0.3.0_darwin_amd64.tar.gz\"\n      sha256 \"cbf170168aa2d3ebdd3e43ac1d31c947ddedee3db0bcf2054a8207e592db9cd4\"\n    end\n  end\n\n  on_linux do\n    on_arm do\n      url \"https://github.com/giacomobartoloni/awl/releases/download/v0.3.0/awl_0.3.0_linux_arm64.tar.gz\"\n      sha256 \"5a879e4d62b7b6d8cb3567f05ce23fe7cd09ddc78842715e879da93ef5bd1677\"\n    end\n\n    on_intel do\n      url \"https://github.com/giacomobartoloni/awl/releases/download/v0.3.0/awl_0.3.0_linux_amd64.tar.gz\"\n      sha256 \"a434b38a6eec89cfff8988092f2acd44f6354ae621aa8c5325aaefce40326ece\"\n    end\n  end\n\n  def install\n    bin.install \"awl\"\n  end\n\n  test do\n    assert_match \"awl #{version}\", shell_output(\"#{bin}/awl --version\")\n    assert_match \"Usage:\", shell_output(\"#{bin}/awl --help\")\n  end\nend\n";
const targets = ["darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64"];
const sums = Object.fromEntries(targets.map((t, i) => [t, String(i + 1).repeat(64)]));
const stable = tag_name => ({ tag_name, draft: false, prerelease: false, published_at: "2026-01-01" });

test("numeric version ordering and stable release selection", () => {
  assert.equal(compareVersions("0.10.0", "0.9.9"), 1);
  assert.equal(latestPublishedRelease([stable("v0.9.9"), stable("v0.10.0"),
    {...stable("v1.0.0"), prerelease:true}, {...stable("v2.0.0"), draft:true},
    stable("v3.0.0-rc.1")]).tag_name, "v0.10.0");
  assert.throws(() => latestPublishedRelease([stable("v0.1.0"), stable("v0.1.0")]), /Duplicate/);
  assert.throws(() => latestPublishedRelease([]), /No eligible/);
});

test("render exactly four consistent URL/checksum pairs, preserve other content", () => {
  const parsed = parseFormula(fixture);
  assert.equal(parsed.version, "0.3.0");
  const updated = updateFormula(fixture, "0.4.0", sums);
  assert.deepEqual(parseFormula(updated), {version:"0.4.0", hashes:sums});
  assert.throws(() => updateFormula(updated, "0.3.0", parsed.hashes), /downgrade/);
  assert.equal(updateFormula(updated, "0.4.0", sums), updated);
  assert.equal(updateFormula(updated.replaceAll("0.4.0", "0.3.0"), "0.3.0", parsed.hashes), fixture);
});

test("reject malformed formula, duplicate targets and README mismatch", () => {
  assert.throws(() => parseFormula(fixture.replace("linux_arm64", "linux_amd64")));
  assert.throws(() => parseFormula(fixture.replace("on_macos", "on_linux")), /layout/);
  assert.throws(() => parseFormula(fixture.replace("awl_0.3.0", "awl_0.2.0")));
  const readme = "The formula installs the official \x60v0.3.0\x60 prebuilt CLI archive with a pinned SHA-256 digest.";
  assert.equal(updateReadme(readme, "0.3.0", "0.4.0"), readme.replace("0.3.0", "0.4.0"));
  assert.throws(() => updateReadme(readme, "0.2.0", "0.4.0"), /README/);
});

function releaseFixture() {
  const release = stable("v0.4.0");
  const payloads = new Map();
  const sha = bytes => createHash("sha256").update(bytes).digest("hex");
  let manifest = "";
  release.assets = targets.map(target => {
    const name = "awl_0.4.0_" + target + ".tar.gz";
    const bytes = Buffer.from(target);
    manifest += sha(bytes) + "  " + name + "\n";
    const url = "https://github.com/giacomobartoloni/awl/releases/download/v0.4.0/" + name;
    payloads.set(url, bytes);
    return {name, browser_download_url:url, size:bytes.length, digest:"sha256:"+sha(bytes)};
  });
  const url = "https://github.com/giacomobartoloni/awl/releases/download/v0.4.0/checksums.txt";
  const bytes = Buffer.from(manifest);
  release.assets.push({name:"checksums.txt", browser_download_url:url, size:bytes.length, digest:"sha256:"+sha(bytes)});
  payloads.set(url, bytes);
  return { release, payloads, fetcher: async url => new Response(payloads.get(url), {status:payloads.has(url)?200:404}) };
}

test("all four downloaded bytes agree with manifest and API metadata", async () => {
  const {release, fetcher} = releaseFixture();
  const hashes = await verifiedReleaseHashes(release, fetcher);
  assert.deepEqual(Object.keys(hashes), targets);
  for (const target of targets) assert.equal(hashes[target], createHash("sha256").update(target).digest("hex"));
});

test("reject missing or duplicate assets, bad digest and tampering", async () => {
  const {release, payloads, fetcher} = releaseFixture();
  assert.throws(() => expectedAssets({...release, assets:release.assets.slice(1)}), /Missing/);
  assert.throws(() => expectedAssets({...release, assets:[...release.assets, release.assets[0]]}), /duplicate/);
  assert.throws(() => parseChecksums("bad"), /Malformed/);
  const a = "a".repeat(64) + "  file\n";
  assert.throws(() => parseChecksums(a + a), /Duplicate/);
  const first = release.assets[0];
  const bad = {...release, assets:[{...first,digest:"sha256:"+"0".repeat(64)}, ...release.assets.slice(1)]};
  await assert.rejects(verifiedReleaseHashes(bad, fetcher), /digest/);
  payloads.set(first.browser_download_url, Buffer.alloc(first.size));
  await assert.rejects(verifiedReleaseHashes(release, fetcher), /checksum/);
});

test("automation ownership guard fails closed on existing unowned branch", async () => {
  const response = (value, status=200) => new Response(JSON.stringify(value), {status});
  await assertAutomationBranchOwnership("token", "tap-app", async () => response({},404));
  await assert.rejects(assertAutomationBranchOwnership("token", "tap-app",
    async url => url.includes("/git/ref/") ? response({}) : response([])), /without one open owned PR/);
  await assert.rejects(assertAutomationBranchOwnership("token", "tap-app",
    async url => url.includes("/git/ref/") ? response({}) : response([{user:{login:"human"}}])), /unexpected owner/);
});
