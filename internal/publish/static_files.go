package publish

import (
	"path"
	"strings"
)

const HeadersFilePath = "_headers"

type ManagedBundleFile struct {
	BundlePath   string
	RequestPaths []string
	MediaType    string
}

var managedBundleFiles = []ManagedBundleFile{
	{
		BundlePath:   ".well-known/did.json",
		RequestPaths: []string{"/.well-known/did.json"},
		MediaType:    "application/did+json",
	},
	{
		BundlePath:   ".well-known/webfinger",
		RequestPaths: []string{"/.well-known/webfinger"},
		MediaType:    "application/json",
	},
	{
		BundlePath:   ".well-known/agent-card.json",
		RequestPaths: []string{"/.well-known/agent-card.json"},
		MediaType:    "application/json",
	},
	{
		BundlePath:   "profile/index.html",
		RequestPaths: []string{"/profile", "/profile/"},
		MediaType:    "text/html; charset=utf-8",
	},
}

func ManagedBundleFiles() []ManagedBundleFile {
	files := make([]ManagedBundleFile, 0, len(managedBundleFiles))
	for _, file := range managedBundleFiles {
		copied := file
		copied.RequestPaths = append([]string(nil), file.RequestPaths...)
		files = append(files, copied)
	}
	return files
}

func ContentTypeForBundlePath(relPath string) (string, bool) {
	normalized := normalizeBundlePath(relPath)
	for _, file := range managedBundleFiles {
		if file.BundlePath == normalized {
			return file.MediaType, true
		}
	}
	return "", false
}

func BuildHeadersFile() []byte {
	var builder strings.Builder
	for _, file := range managedBundleFiles {
		for _, requestPath := range file.RequestPaths {
			builder.WriteString(requestPath)
			builder.WriteString("\n")
			builder.WriteString("  Content-Type: ")
			builder.WriteString(file.MediaType)
			builder.WriteString("\n")
			builder.WriteString("  X-Content-Type-Options: nosniff\n\n")
		}
	}
	return []byte(strings.TrimSpace(builder.String()) + "\n")
}

func normalizeBundlePath(relPath string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(relPath), "\\", "/")
	if normalized == "" {
		return ""
	}
	return strings.TrimPrefix(path.Clean("/"+normalized), "/")
}
