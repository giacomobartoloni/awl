package awl

import (
	"os"
	"path/filepath"
	"strings"
)

func configRead(file string, allowed map[string]bool) (map[string]string, error) {
	b, e := os.ReadFile(file)
	if e != nil {
		return nil, e
	}
	m := map[string]string{}
	scope := ""
	seenSections := map[string]bool{}
	for _, raw := range strings.Split(string(b), "\n") {
		// Accept CRLF as well as LF without rewriting the user's config bytes.
		raw = strings.TrimSuffix(raw, "\r")
		s := strings.TrimSpace(raw)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if strings.HasSuffix(s, ":") && raw == s {
			scope = strings.TrimSuffix(s, ":")
			if seenSections[scope] {
				return nil, fail("INVALID_CONFIG", "duplicate YAML section")
			}
			seenSections[scope] = true
			if !allowed[scope] {
				return nil, fail("INVALID_CONFIG", "unknown config section")
			}
			continue
		}
		name, v, ok := strings.Cut(s, ":")
		if !ok {
			return nil, fail("INVALID_CONFIG", "malformed YAML field")
		}
		key := strings.TrimSpace(name)
		if strings.HasPrefix(raw, "  ") && scope != "" {
			key = scope + "." + key
		} else if raw != s {
			return nil, fail("INVALID_CONFIG", "unsupported YAML indentation")
		} else {
			scope = ""
		}
		if !allowed[key] {
			return nil, fail("INVALID_CONFIG", "unknown YAML field: "+key)
		}
		if _, duplicate := m[key]; duplicate {
			return nil, fail("INVALID_CONFIG", "duplicate YAML field: "+key)
		}
		value := strings.TrimSpace(v)
		if value == "" || strings.ContainsAny(value, "\n\r#\"'{}[]") || strings.HasPrefix(value, "!") {
			return nil, fail("INVALID_CONFIG", "unsupported YAML scalar: "+key)
		}
		m[key] = value
	}
	if m["schema"] != "1" {
		return nil, fail("INVALID_CONFIG", "expected schema: 1")
	}
	return m, nil
}
func parseShared(path string) (string, string, error) {
	slug, remote, _, e := parseSharedTarget(path)
	return slug, remote, e
}

func parseSharedTarget(path string) (string, string, string, error) {
	m, e := configRead(path, map[string]bool{
		"project": true, "storage": true, "schema": true,
		"project.slug": true, "storage.remote": true, "storage.remoteBranch": true,
	})
	if e != nil {
		return "", "", "", e
	}
	slug, remote := m["project.slug"], m["storage.remote"]
	if !slugPattern.MatchString(slug) {
		return "", "", "", fail("INVALID_CONFIG", "missing or invalid project slug")
	}
	if e = validRemote(remote); e != nil {
		return "", "", "", e
	}
	branch := m["storage.remoteBranch"]
	if branch != "" && remote == "" {
		return "", "", "", fail("INVALID_CONFIG", "remoteBranch requires a remote URL")
	}
	if branch == "" {
		branch = defaultRemoteBranch
	}
	if e = validRemoteBranch(branch); e != nil {
		return "", "", "", e
	}
	return slug, remote, branch, nil
}

func parseLocal(path string) (binding, error) {
	m, e := configRead(path, map[string]bool{
		"binding": true, "storage": true, "schema": true,
		"binding.slug": true, "binding.remote": true, "binding.remoteBranch": true,
		"storage.localRepo": true,
	})
	if e != nil {
		return binding{}, e
	}
	b := binding{
		Slug: m["binding.slug"], Remote: m["binding.remote"],
		RemoteBranch: m["binding.remoteBranch"], LocalRepo: m["storage.localRepo"],
	}
	if !slugPattern.MatchString(b.Slug) || !filepath.IsAbs(b.LocalRepo) {
		return b, fail("INVALID_CONFIG", "incomplete local binding")
	}
	if e = validRemote(b.Remote); e != nil {
		return b, e
	}
	if b.RemoteBranch != "" && b.Remote == "" {
		return b, fail("INVALID_CONFIG", "remoteBranch requires a remote URL")
	}
	if b.RemoteBranch == "" {
		b.RemoteBranch = defaultRemoteBranch
	}
	if e = validRemoteBranch(b.RemoteBranch); e != nil {
		return b, e
	}
	return b, nil
}
