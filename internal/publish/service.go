package publish

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/layout"

	_ "modernc.org/sqlite"
)

const (
	TierMinimum     = "minimum"
	TierRecommended = "recommended"
	TierFull        = "full"
)

type artifactKind string

const (
	artifactDID       artifactKind = "did"
	artifactWebFinger artifactKind = "webfinger"
	artifactAgentCard artifactKind = "agent-card"
	artifactProfile   artifactKind = "profile"
)

var managedArtifactKinds = []artifactKind{
	artifactDID,
	artifactWebFinger,
	artifactAgentCard,
	artifactProfile,
}

type Options struct {
	Home   string
	Origin string
	Output string
	Tier   string
}

type Result struct {
	Home         string     `json:"home"`
	DBPath       string     `json:"db_path"`
	OutputDir    string     `json:"output_dir"`
	Tier         string     `json:"tier"`
	CanonicalID  string     `json:"canonical_id"`
	HomeOrigin   string     `json:"home_origin"`
	Artifacts    []Artifact `json:"artifacts"`
	Checks       []Check    `json:"checks"`
	ManifestPath string     `json:"manifest_path"`
	GeneratedAt  string     `json:"generated_at"`
}

type Artifact struct {
	Type      string `json:"type"`
	Path      string `json:"path"`
	URL       string `json:"url"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
}

type Check struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Details string `json:"details"`
}

type Manifest struct {
	GeneratedAt   string            `json:"generated_at"`
	Tier          string            `json:"tier"`
	CanonicalID   string            `json:"canonical_id"`
	HomeOrigin    string            `json:"home_origin"`
	Artifacts     []Artifact        `json:"artifacts"`
	ContentHashes map[string]string `json:"content_hashes"`
	Checks        []Check           `json:"checks"`
}

type Service struct {
	Now func() time.Time
}

type selfIdentity struct {
	SelfID            string
	CanonicalID       string
	DisplayName       string
	Description       string
	HomeOrigin        string
	DefaultProfileURL string
}

type keyRecord struct {
	KeyID           string
	Algorithm       string
	PublicKey       string
	Status          string
	PublishedStatus string
	ValidFrom       string
	ValidUntil      string
	RetiredAt       string
	RevokedAt       string
	GraceUntil      string
}

type bundleURLs struct {
	Origin    string
	Resource  string
	DID       string
	WebFinger string
	AgentCard string
	Profile   string
}

type compiledBundle struct {
	DID         didDocument
	WebFinger   webFingerDocument
	AgentCard   agentCardDocument
	ProfileHTML string
}

type didDocument struct {
	Context            []string                `json:"@context"`
	ID                 string                  `json:"id"`
	AlsoKnownAs        []string                `json:"alsoKnownAs,omitempty"`
	VerificationMethod []didVerificationMethod `json:"verificationMethod"`
	Authentication     []string                `json:"authentication,omitempty"`
	AssertionMethod    []string                `json:"assertionMethod,omitempty"`
	Service            []didService            `json:"service,omitempty"`
}

type didVerificationMethod struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	Controller      string          `json:"controller"`
	PublicKeyJWK    didPublicKeyJWK `json:"publicKeyJwk"`
	KeyStatus       string          `json:"key_status"`
	PublishedStatus string          `json:"published_status"`
	ValidFrom       string          `json:"valid_from"`
	ValidUntil      string          `json:"valid_until,omitempty"`
	RetiredAt       string          `json:"retired_at,omitempty"`
	RevokedAt       string          `json:"revoked_at,omitempty"`
	GraceUntil      string          `json:"grace_until,omitempty"`
}

type didPublicKeyJWK struct {
	KTY string `json:"kty"`
	CRV string `json:"crv"`
	X   string `json:"x"`
}

type didService struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	ServiceEndpoint string `json:"serviceEndpoint"`
}

type webFingerDocument struct {
	Subject string          `json:"subject"`
	Aliases []string        `json:"aliases,omitempty"`
	Links   []webFingerLink `json:"links"`
}

type webFingerLink struct {
	Rel  string `json:"rel"`
	Type string `json:"type,omitempty"`
	Href string `json:"href"`
}

type agentCardDocument struct {
	ID                  string   `json:"id"`
	CanonicalID         string   `json:"canonical_id"`
	Name                string   `json:"name"`
	Description         string   `json:"description,omitempty"`
	Origin              string   `json:"origin"`
	ServiceEndpoint     string   `json:"service_endpoint"`
	DIDURL              string   `json:"did_url"`
	WebFingerURL        string   `json:"webfinger_url"`
	ProfileURL          string   `json:"profile_url,omitempty"`
	VerificationMethods []string `json:"verification_methods"`
	Capabilities        []string `json:"capabilities"`
	AuthRequirements    []string `json:"auth_requirements"`
}

var profileTemplate = template.Must(template.New("profile").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Name}} | LinkClaw Identity</title>
</head>
<body>
  <main>
    <h1>{{.Name}}</h1>
    <p>Canonical ID: <code>{{.CanonicalID}}</code></p>
    {{if .Description}}<p>{{.Description}}</p>{{end}}
    <ul>
      <li><a href="{{.DIDURL}}">did.json</a></li>
      <li><a href="{{.WebFingerURL}}">webfinger</a></li>
      <li><a href="{{.AgentCardURL}}">agent-card.json</a></li>
    </ul>
  </main>
</body>
</html>
`))

func NewService() *Service {
	return &Service{Now: time.Now}
}

func (s *Service) Publish(ctx context.Context, opts Options) (Result, error) {
	tier, err := normalizeTier(opts.Tier)
	if err != nil {
		return Result{}, err
	}

	home, err := layout.ResolveHome(opts.Home)
	if err != nil {
		return Result{}, err
	}
	paths := layout.BuildPaths(home)
	if _, err := os.Stat(paths.DB); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{}, fmt.Errorf("state db not found at %q; run linkclaw init first", paths.DB)
		}
		return Result{}, fmt.Errorf("stat state db: %w", err)
	}

	outputDir, err := resolveOutput(home, opts.Output)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create output directory: %w", err)
	}

	db, err := sql.Open("sqlite", paths.DB)
	if err != nil {
		return Result{}, fmt.Errorf("open sqlite database: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return Result{}, fmt.Errorf("ping sqlite database: %w", err)
	}

	identity, err := loadSelfIdentity(ctx, db)
	if err != nil {
		return Result{}, err
	}

	origin, err := resolveOrigin(opts.Origin, identity.HomeOrigin)
	if err != nil {
		return Result{}, err
	}
	urls := buildBundleURLs(origin)

	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	generatedAt := nowFn().UTC().Format(time.RFC3339Nano)

	if err := persistIdentityURLs(ctx, db, identity, urls, generatedAt); err != nil {
		return Result{}, err
	}
	identity.HomeOrigin = urls.Origin
	identity.DefaultProfileURL = urls.Profile

	keys, err := loadKeys(ctx, db, identity.SelfID)
	if err != nil {
		return Result{}, err
	}

	selectedKinds := tierArtifacts(tier)
	bundle, err := compileBundle(identity, keys, urls, selectedKinds)
	if err != nil {
		return Result{}, err
	}
	checks := evaluateChecks(identity, urls, bundle, selectedKinds)

	if err := removeStaleManagedFiles(outputDir, selectedKinds); err != nil {
		return Result{}, err
	}

	artifacts := make([]Artifact, 0, len(selectedKinds))
	contentHashes := make(map[string]string, len(selectedKinds))
	for _, kind := range selectedKinds {
		content, err := renderArtifact(kind, bundle)
		if err != nil {
			return Result{}, err
		}

		artifact := Artifact{
			Type:      kind.typeName(),
			Path:      kind.relPath(),
			URL:       urls.urlFor(kind),
			MediaType: kind.mediaType(),
			SHA256:    digest(content),
		}
		if err := writeBundleFile(outputDir, artifact.Path, content); err != nil {
			return Result{}, err
		}
		artifacts = append(artifacts, artifact)
		contentHashes[artifact.Path] = artifact.SHA256
	}

	manifestPath := filepath.Join(outputDir, "manifest.json")
	manifest := Manifest{
		GeneratedAt:   generatedAt,
		Tier:          tier,
		CanonicalID:   identity.CanonicalID,
		HomeOrigin:    urls.Origin,
		Artifacts:     artifacts,
		ContentHashes: contentHashes,
		Checks:        checks,
	}
	manifestContent, err := marshalJSON(manifest)
	if err != nil {
		return Result{}, fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestContent, 0o644); err != nil {
		return Result{}, fmt.Errorf("write manifest: %w", err)
	}

	if failed := failedChecks(checks); len(failed) > 0 {
		return Result{
			Home:         home,
			DBPath:       paths.DB,
			OutputDir:    outputDir,
			Tier:         tier,
			CanonicalID:  identity.CanonicalID,
			HomeOrigin:   urls.Origin,
			Artifacts:    artifacts,
			Checks:       checks,
			ManifestPath: manifestPath,
			GeneratedAt:  generatedAt,
		}, fmt.Errorf("bundle consistency checks failed: %s", strings.Join(failed, ", "))
	}

	return Result{
		Home:         home,
		DBPath:       paths.DB,
		OutputDir:    outputDir,
		Tier:         tier,
		CanonicalID:  identity.CanonicalID,
		HomeOrigin:   urls.Origin,
		Artifacts:    artifacts,
		Checks:       checks,
		ManifestPath: manifestPath,
		GeneratedAt:  generatedAt,
	}, nil
}

func normalizeTier(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", TierRecommended:
		return TierRecommended, nil
	case TierMinimum, TierFull:
		return strings.TrimSpace(raw), nil
	default:
		return "", fmt.Errorf("unsupported publish tier %q", raw)
	}
}

func resolveOutput(home, explicit string) (string, error) {
	if strings.TrimSpace(explicit) == "" {
		return filepath.Join(home, "publish"), nil
	}
	abs, err := filepath.Abs(explicit)
	if err != nil {
		return "", fmt.Errorf("resolve output directory: %w", err)
	}
	return abs, nil
}

func loadSelfIdentity(ctx context.Context, db *sql.DB) (selfIdentity, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT self_id, canonical_id, display_name, description, home_origin, default_profile_url
		FROM self_identities
		WHERE status = 'active'
		ORDER BY created_at ASC
		LIMIT 2
	`)
	if err != nil {
		return selfIdentity{}, fmt.Errorf("query self identity: %w", err)
	}
	defer rows.Close()

	var identities []selfIdentity
	for rows.Next() {
		var identity selfIdentity
		if err := rows.Scan(
			&identity.SelfID,
			&identity.CanonicalID,
			&identity.DisplayName,
			&identity.Description,
			&identity.HomeOrigin,
			&identity.DefaultProfileURL,
		); err != nil {
			return selfIdentity{}, fmt.Errorf("scan self identity: %w", err)
		}
		identities = append(identities, identity)
	}
	if err := rows.Err(); err != nil {
		return selfIdentity{}, fmt.Errorf("iterate self identities: %w", err)
	}
	switch len(identities) {
	case 0:
		return selfIdentity{}, errors.New("no active self identity found; run linkclaw init first")
	case 1:
		return identities[0], nil
	default:
		return selfIdentity{}, errors.New("multiple active self identities found; publish currently supports exactly one")
	}
}

func resolveOrigin(explicit, stored string) (string, error) {
	raw := strings.TrimSpace(explicit)
	if raw == "" {
		raw = strings.TrimSpace(stored)
	}
	if raw == "" {
		return "", errors.New("home origin is required (set --origin on first publish)")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse origin: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("origin %q must include scheme and host", raw)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("origin %q must use http or https", raw)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("origin %q must not include query or fragment", raw)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("origin %q must not include a path", raw)
	}

	host := strings.ToLower(parsed.Hostname())
	if port := parsed.Port(); port != "" {
		host = host + ":" + port
	}
	return scheme + "://" + host, nil
}

func buildBundleURLs(origin string) bundleURLs {
	return bundleURLs{
		Origin:    origin,
		Resource:  origin + "/",
		DID:       origin + "/.well-known/did.json",
		WebFinger: origin + "/.well-known/webfinger",
		AgentCard: origin + "/.well-known/agent-card.json",
		Profile:   origin + "/profile/",
	}
}

func persistIdentityURLs(ctx context.Context, db *sql.DB, identity selfIdentity, urls bundleURLs, updatedAt string) error {
	if identity.HomeOrigin == urls.Origin && identity.DefaultProfileURL == urls.Profile {
		return nil
	}
	if _, err := db.ExecContext(
		ctx,
		"UPDATE self_identities SET home_origin = ?, default_profile_url = ?, updated_at = ? WHERE self_id = ?",
		urls.Origin,
		urls.Profile,
		updatedAt,
		identity.SelfID,
	); err != nil {
		return fmt.Errorf("update self identity publish urls: %w", err)
	}
	return nil
}

func loadKeys(ctx context.Context, db *sql.DB, selfID string) ([]keyRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key_id, algorithm, public_key, status, published_status, valid_from, valid_until, retired_at, revoked_at, grace_until
		FROM keys
		WHERE owner_type = 'self' AND owner_id = ? AND status IN ('active', 'retired')
		ORDER BY created_at ASC
	`, selfID)
	if err != nil {
		return nil, fmt.Errorf("query self keys: %w", err)
	}
	defer rows.Close()

	keys := make([]keyRecord, 0)
	for rows.Next() {
		var key keyRecord
		if err := rows.Scan(
			&key.KeyID,
			&key.Algorithm,
			&key.PublicKey,
			&key.Status,
			&key.PublishedStatus,
			&key.ValidFrom,
			&key.ValidUntil,
			&key.RetiredAt,
			&key.RevokedAt,
			&key.GraceUntil,
		); err != nil {
			return nil, fmt.Errorf("scan self key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate self keys: %w", err)
	}
	if len(keys) == 0 {
		return nil, errors.New("no publishable self keys found; run linkclaw init first")
	}
	if len(activeKeyRefs(keys, "")) == 0 {
		return nil, errors.New("no active self keys found for publish")
	}
	return keys, nil
}

func tierArtifacts(tier string) []artifactKind {
	switch tier {
	case TierMinimum:
		return []artifactKind{artifactDID}
	case TierFull:
		return []artifactKind{artifactDID, artifactWebFinger, artifactAgentCard, artifactProfile}
	default:
		return []artifactKind{artifactDID, artifactWebFinger, artifactAgentCard}
	}
}

func compileBundle(identity selfIdentity, keys []keyRecord, urls bundleURLs, selected []artifactKind) (compiledBundle, error) {
	selectedSet := make(map[artifactKind]bool, len(selected))
	for _, kind := range selected {
		selectedSet[kind] = true
	}

	did, err := buildDIDDocument(identity, keys, urls, selectedSet)
	if err != nil {
		return compiledBundle{}, err
	}
	bundle := compiledBundle{DID: did}

	if selectedSet[artifactWebFinger] {
		bundle.WebFinger = buildWebFingerDocument(identity, urls, selectedSet)
	}
	if selectedSet[artifactAgentCard] {
		bundle.AgentCard = buildAgentCardDocument(identity, keys, urls, selectedSet)
	}
	if selectedSet[artifactProfile] {
		profileHTML, err := renderProfile(identity, urls)
		if err != nil {
			return compiledBundle{}, err
		}
		bundle.ProfileHTML = profileHTML
	}

	return bundle, nil
}

func buildDIDDocument(identity selfIdentity, keys []keyRecord, urls bundleURLs, selected map[artifactKind]bool) (didDocument, error) {
	verificationMethods := make([]didVerificationMethod, 0, len(keys))
	authentication := make([]string, 0, len(keys))
	assertionMethod := make([]string, 0, len(keys))

	for _, key := range keys {
		vm, err := toVerificationMethod(identity.CanonicalID, key)
		if err != nil {
			return didDocument{}, err
		}
		verificationMethods = append(verificationMethods, vm)
		if key.Status == "active" {
			authentication = append(authentication, vm.ID)
			assertionMethod = append(assertionMethod, vm.ID)
		}
	}

	services := []didService{
		{
			ID:              identity.CanonicalID + "#home",
			Type:            "LinkClawHome",
			ServiceEndpoint: urls.Resource,
		},
	}
	if selected[artifactWebFinger] {
		services = append(services, didService{
			ID:              identity.CanonicalID + "#webfinger",
			Type:            "WebFinger",
			ServiceEndpoint: urls.WebFinger,
		})
	}
	if selected[artifactAgentCard] {
		services = append(services, didService{
			ID:              identity.CanonicalID + "#agent-card",
			Type:            "AgentCard",
			ServiceEndpoint: urls.AgentCard,
		})
	}
	if selected[artifactProfile] {
		services = append(services, didService{
			ID:              identity.CanonicalID + "#profile",
			Type:            "ProfilePage",
			ServiceEndpoint: urls.Profile,
		})
	}

	alsoKnownAs := []string{urls.Resource}
	if selected[artifactProfile] {
		alsoKnownAs = append(alsoKnownAs, urls.Profile)
	}

	return didDocument{
		Context:            []string{"https://www.w3.org/ns/did/v1"},
		ID:                 identity.CanonicalID,
		AlsoKnownAs:        alsoKnownAs,
		VerificationMethod: verificationMethods,
		Authentication:     authentication,
		AssertionMethod:    assertionMethod,
		Service:            services,
	}, nil
}

func toVerificationMethod(canonicalID string, key keyRecord) (didVerificationMethod, error) {
	publicBytes, err := base64.RawStdEncoding.DecodeString(key.PublicKey)
	if err != nil {
		return didVerificationMethod{}, fmt.Errorf("decode public key %q: %w", key.KeyID, err)
	}
	return didVerificationMethod{
		ID:         canonicalID + "#" + key.KeyID,
		Type:       "JsonWebKey2020",
		Controller: canonicalID,
		PublicKeyJWK: didPublicKeyJWK{
			KTY: "OKP",
			CRV: "Ed25519",
			X:   base64.RawURLEncoding.EncodeToString(publicBytes),
		},
		KeyStatus:       key.Status,
		PublishedStatus: key.PublishedStatus,
		ValidFrom:       key.ValidFrom,
		ValidUntil:      key.ValidUntil,
		RetiredAt:       key.RetiredAt,
		RevokedAt:       key.RevokedAt,
		GraceUntil:      key.GraceUntil,
	}, nil
}

func buildWebFingerDocument(identity selfIdentity, urls bundleURLs, selected map[artifactKind]bool) webFingerDocument {
	links := []webFingerLink{
		{
			Rel:  "self",
			Type: "application/did+json",
			Href: urls.DID,
		},
	}
	if selected[artifactAgentCard] {
		links = append(links, webFingerLink{
			Rel:  "service-desc",
			Type: "application/json",
			Href: urls.AgentCard,
		})
	}
	if selected[artifactProfile] {
		links = append(links, webFingerLink{
			Rel:  "profile-page",
			Type: "text/html",
			Href: urls.Profile,
		})
	}

	return webFingerDocument{
		Subject: urls.Resource,
		Aliases: []string{identity.CanonicalID},
		Links:   links,
	}
}

func buildAgentCardDocument(identity selfIdentity, keys []keyRecord, urls bundleURLs, selected map[artifactKind]bool) agentCardDocument {
	card := agentCardDocument{
		ID:                  urls.AgentCard,
		CanonicalID:         identity.CanonicalID,
		Name:                identity.DisplayName,
		Description:         identity.Description,
		Origin:              urls.Origin,
		ServiceEndpoint:     urls.Resource,
		DIDURL:              urls.DID,
		WebFingerURL:        urls.WebFinger,
		VerificationMethods: activeKeyRefs(keys, identity.CanonicalID),
		Capabilities:        []string{},
		AuthRequirements:    []string{"Requests should authenticate with one of the active did.json verification methods."},
	}
	if selected[artifactProfile] {
		card.ProfileURL = urls.Profile
	}
	return card
}

func activeKeyRefs(keys []keyRecord, canonicalID string) []string {
	refs := make([]string, 0, len(keys))
	for _, key := range keys {
		if key.Status != "active" {
			continue
		}
		ref := key.KeyID
		if canonicalID != "" {
			ref = canonicalID + "#" + key.KeyID
		}
		refs = append(refs, ref)
	}
	return refs
}

func renderProfile(identity selfIdentity, urls bundleURLs) (string, error) {
	data := struct {
		Name         string
		CanonicalID  string
		Description  string
		DIDURL       string
		WebFingerURL string
		AgentCardURL string
	}{
		Name:         identity.DisplayName,
		CanonicalID:  identity.CanonicalID,
		Description:  identity.Description,
		DIDURL:       urls.DID,
		WebFingerURL: urls.WebFinger,
		AgentCardURL: urls.AgentCard,
	}

	var buf bytes.Buffer
	if err := profileTemplate.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render profile: %w", err)
	}
	return buf.String(), nil
}

func evaluateChecks(identity selfIdentity, urls bundleURLs, bundle compiledBundle, selected []artifactKind) []Check {
	selectedSet := make(map[artifactKind]bool, len(selected))
	for _, kind := range selected {
		selectedSet[kind] = true
	}

	checks := []Check{
		{
			Name:    "did-canonical-id",
			OK:      bundle.DID.ID == identity.CanonicalID,
			Details: "did.json id must match canonical_id",
		},
		{
			Name:    "did-active-verification-methods",
			OK:      len(bundle.DID.Authentication) > 0 && refsCovered(bundle.DID.Authentication, verificationMethodSet(bundle.DID.VerificationMethod)),
			Details: "did.json must publish at least one active verification method",
		},
		{
			Name:    "did-service-origin",
			OK:      servicesStayOnOrigin(bundle.DID.Service, urls.Origin),
			Details: "did.json service endpoints must stay on the selected home origin",
		},
	}

	if selectedSet[artifactWebFinger] {
		checks = append(checks, Check{
			Name:    "webfinger-cross-links",
			OK:      bundle.WebFinger.Subject == urls.Resource && webFingerHasLink(bundle.WebFinger.Links, "self", urls.DID) && (!selectedSet[artifactAgentCard] || webFingerHasLink(bundle.WebFinger.Links, "service-desc", urls.AgentCard)) && (!selectedSet[artifactProfile] || webFingerHasLink(bundle.WebFinger.Links, "profile-page", urls.Profile)),
			Details: "webfinger must resolve the origin resource and point back to the published did/profile/card artifacts",
		})
	}

	if selectedSet[artifactAgentCard] {
		checks = append(checks, Check{
			Name:    "agent-card-precedence",
			OK:      bundle.AgentCard.CanonicalID == identity.CanonicalID && bundle.AgentCard.DIDURL == urls.DID && refsCovered(bundle.AgentCard.VerificationMethods, stringSet(bundle.DID.Authentication)),
			Details: "agent-card.json must reference the canonical did and only active did.json keys",
		})
	}

	if selectedSet[artifactProfile] {
		checks = append(checks, Check{
			Name:    "profile-links",
			OK:      strings.Contains(bundle.ProfileHTML, urls.DID) && strings.Contains(bundle.ProfileHTML, urls.WebFinger) && strings.Contains(bundle.ProfileHTML, urls.AgentCard),
			Details: "profile page must link to did.json, webfinger, and agent-card.json",
		})
	}

	return checks
}

func servicesStayOnOrigin(services []didService, origin string) bool {
	for _, service := range services {
		if !strings.HasPrefix(service.ServiceEndpoint, origin) {
			return false
		}
	}
	return true
}

func webFingerHasLink(links []webFingerLink, rel, href string) bool {
	for _, link := range links {
		if link.Rel == rel && link.Href == href {
			return true
		}
	}
	return false
}

func verificationMethodSet(methods []didVerificationMethod) map[string]struct{} {
	set := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		set[method.ID] = struct{}{}
	}
	return set
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func refsCovered(refs []string, set map[string]struct{}) bool {
	if len(refs) == 0 {
		return false
	}
	for _, ref := range refs {
		if _, ok := set[ref]; !ok {
			return false
		}
	}
	return true
}

func removeStaleManagedFiles(outputDir string, selected []artifactKind) error {
	selectedSet := make(map[artifactKind]bool, len(selected))
	for _, kind := range selected {
		selectedSet[kind] = true
	}
	for _, kind := range managedArtifactKinds {
		if selectedSet[kind] {
			continue
		}
		path := filepath.Join(outputDir, filepath.FromSlash(kind.relPath()))
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale artifact %q: %w", kind.relPath(), err)
		}
	}
	return nil
}

func renderArtifact(kind artifactKind, bundle compiledBundle) ([]byte, error) {
	switch kind {
	case artifactDID:
		return marshalJSON(bundle.DID)
	case artifactWebFinger:
		return marshalJSON(bundle.WebFinger)
	case artifactAgentCard:
		return marshalJSON(bundle.AgentCard)
	case artifactProfile:
		return []byte(bundle.ProfileHTML), nil
	default:
		return nil, fmt.Errorf("unsupported artifact type %q", kind)
	}
}

func marshalJSON(v any) ([]byte, error) {
	content, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

func writeBundleFile(outputDir, relPath string, content []byte) error {
	absPath := filepath.Join(outputDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("create artifact directory for %q: %w", relPath, err)
	}
	if err := os.WriteFile(absPath, content, 0o644); err != nil {
		return fmt.Errorf("write artifact %q: %w", relPath, err)
	}
	return nil
}

func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func failedChecks(checks []Check) []string {
	failed := make([]string, 0)
	for _, check := range checks {
		if !check.OK {
			failed = append(failed, check.Name)
		}
	}
	return failed
}

func (k artifactKind) relPath() string {
	switch k {
	case artifactDID:
		return ".well-known/did.json"
	case artifactWebFinger:
		return ".well-known/webfinger"
	case artifactAgentCard:
		return ".well-known/agent-card.json"
	case artifactProfile:
		return "profile/index.html"
	default:
		return ""
	}
}

func (k artifactKind) typeName() string {
	return string(k)
}

func (k artifactKind) mediaType() string {
	switch k {
	case artifactDID:
		return "application/did+json"
	case artifactWebFinger, artifactAgentCard:
		return "application/json"
	case artifactProfile:
		return "text/html"
	default:
		return "application/octet-stream"
	}
}

func (u bundleURLs) urlFor(kind artifactKind) string {
	switch kind {
	case artifactDID:
		return u.DID
	case artifactWebFinger:
		return u.WebFinger
	case artifactAgentCard:
		return u.AgentCard
	case artifactProfile:
		return u.Profile
	default:
		return u.Origin
	}
}
