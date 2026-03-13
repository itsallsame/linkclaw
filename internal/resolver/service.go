package resolver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	StatusDiscovered = "discovered"
	StatusResolved   = "resolved"
	StatusConsistent = "consistent"
	StatusMismatch   = "mismatch"
)

const (
	artifactDID       = "did"
	artifactWebFinger = "webfinger"
	artifactAgentCard = "agent-card"
	artifactProfile   = "profile"
)

var (
	hrefPattern         = regexp.MustCompile(`(?is)href\s*=\s*["']([^"']+)["']`)
	h1Pattern           = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)
	profileIDCodeRegexp = regexp.MustCompile(`(?is)Canonical ID:\s*<code>(.*?)</code>`)
)

type Service struct {
	Client *http.Client
	Now    func() time.Time
}

type Result struct {
	Input            string     `json:"input"`
	NormalizedOrigin string     `json:"normalized_origin,omitempty"`
	Resource         string     `json:"resource,omitempty"`
	Status           string     `json:"status"`
	CanonicalID      string     `json:"canonical_id,omitempty"`
	DisplayName      string     `json:"display_name,omitempty"`
	ProfileURL       string     `json:"profile_url,omitempty"`
	Artifacts        []Artifact `json:"artifacts"`
	Proofs           []Proof    `json:"proofs,omitempty"`
	Mismatches       []string   `json:"mismatches,omitempty"`
	Warnings         []string   `json:"warnings,omitempty"`
	ResolvedAt       string     `json:"resolved_at"`
}

type Artifact struct {
	Type        string `json:"type"`
	URL         string `json:"url"`
	HTTPStatus  int    `json:"http_status"`
	OK          bool   `json:"ok"`
	ContentHash string `json:"content_hash,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Error       string `json:"error,omitempty"`
}

type Proof struct {
	Type           string `json:"type"`
	URL            string `json:"url"`
	ObservedValue  string `json:"observed_value"`
	VerifiedStatus string `json:"verified_status"`
}

type didDocument struct {
	ID                 string                  `json:"id"`
	AlsoKnownAs        []string                `json:"alsoKnownAs"`
	VerificationMethod []didVerificationMethod `json:"verificationMethod"`
	Authentication     []string                `json:"authentication"`
	Service            []didService            `json:"service"`
}

type didVerificationMethod struct {
	ID string `json:"id"`
}

type didService struct {
	Type            string `json:"type"`
	ServiceEndpoint string `json:"serviceEndpoint"`
}

type webFingerDocument struct {
	Subject string          `json:"subject"`
	Aliases []string        `json:"aliases"`
	Links   []webFingerLink `json:"links"`
}

type webFingerLink struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}

type agentCardDocument struct {
	CanonicalID         string   `json:"canonical_id"`
	Name                string   `json:"name"`
	Origin              string   `json:"origin"`
	DIDURL              string   `json:"did_url"`
	WebFingerURL        string   `json:"webfinger_url"`
	ProfileURL          string   `json:"profile_url"`
	VerificationMethods []string `json:"verification_methods"`
}

type profileDocument struct {
	Name        string
	CanonicalID string
	Links       []string
}

type normalizedInput struct {
	Raw          string
	Origin       string
	Resource     string
	DIDURL       string
	WebFingerURL string
	AgentCardURL string
	ProfileURL   string
}

type fetchResult struct {
	Artifact Artifact
	Body     []byte
}

func NewService() *Service {
	return &Service{
		Client: &http.Client{Timeout: 10 * time.Second},
		Now:    time.Now,
	}
}

func (s *Service) Inspect(ctx context.Context, rawInput string) (Result, error) {
	target, err := normalizeInput(rawInput)
	if err != nil {
		return Result{}, err
	}

	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	didFetch := fetchResult{Artifact: Artifact{Type: artifactDID, URL: target.DIDURL}}
	if target.DIDURL != "" {
		didFetch = fetchArtifact(ctx, client, artifactDID, target.DIDURL)
	}
	var didDoc *didDocument
	if didFetch.Artifact.OK {
		doc, parseErr := parseDID(didFetch.Body)
		if parseErr == nil {
			didDoc = doc
			didFetch.Artifact.Summary = summarizeDID(doc)
			target = adoptDIDServices(target, doc)
		} else {
			didFetch.Artifact.OK = false
			didFetch.Artifact.Error = parseErr.Error()
		}
	}

	cardFetch := fetchArtifact(ctx, client, artifactAgentCard, target.AgentCardURL)
	var cardDoc *agentCardDocument
	if cardFetch.Artifact.OK {
		doc, parseErr := parseAgentCard(cardFetch.Body)
		if parseErr == nil {
			cardDoc = doc
			cardFetch.Artifact.Summary = summarizeAgentCard(doc)
			if didDoc == nil && doc.DIDURL != "" && !sameURL(doc.DIDURL, target.DIDURL) {
				didFetch = fetchArtifact(ctx, client, artifactDID, doc.DIDURL)
				if didFetch.Artifact.OK {
					didDoc, parseErr = parseDID(didFetch.Body)
					if parseErr == nil {
						didFetch.Artifact.Summary = summarizeDID(didDoc)
						target = adoptDIDServices(target, didDoc)
					} else {
						didFetch.Artifact.OK = false
						didFetch.Artifact.Error = parseErr.Error()
						didDoc = nil
					}
				}
			}
			if doc.WebFingerURL != "" {
				target.WebFingerURL = withWebFingerResource(doc.WebFingerURL, target.Resource)
			}
			if doc.ProfileURL != "" {
				target.ProfileURL = doc.ProfileURL
			}
		} else {
			cardFetch.Artifact.OK = false
			cardFetch.Artifact.Error = parseErr.Error()
		}
	}

	webFingerFetch := fetchArtifact(ctx, client, artifactWebFinger, target.WebFingerURL)
	var webFingerDoc *webFingerDocument
	if webFingerFetch.Artifact.OK {
		doc, parseErr := parseWebFinger(webFingerFetch.Body)
		if parseErr == nil {
			webFingerDoc = doc
			webFingerFetch.Artifact.Summary = summarizeWebFinger(doc)
		} else {
			webFingerFetch.Artifact.OK = false
			webFingerFetch.Artifact.Error = parseErr.Error()
		}
	}

	profileFetch := fetchArtifact(ctx, client, artifactProfile, target.ProfileURL)
	var profileDoc *profileDocument
	if profileFetch.Artifact.OK {
		doc := parseProfile(profileFetch.Body, target.Origin)
		profileDoc = &doc
		profileFetch.Artifact.Summary = summarizeProfile(doc)
	}

	artifacts := []Artifact{
		didFetch.Artifact,
		webFingerFetch.Artifact,
		cardFetch.Artifact,
		profileFetch.Artifact,
	}
	artifacts = compactArtifacts(artifacts)

	if countSuccessfulArtifacts(artifacts) == 0 {
		return Result{}, fmt.Errorf("no public identity artifacts found for %q", strings.TrimSpace(rawInput))
	}

	result := Result{
		Input:            strings.TrimSpace(rawInput),
		NormalizedOrigin: target.Origin,
		Resource:         target.Resource,
		Status:           StatusDiscovered,
		Artifacts:        artifacts,
		ResolvedAt:       nowFn().UTC().Format(time.RFC3339Nano),
	}

	if didDoc != nil {
		result.CanonicalID = strings.TrimSpace(didDoc.ID)
		result.Status = StatusResolved
	}
	if cardDoc != nil && strings.TrimSpace(cardDoc.Name) != "" {
		result.DisplayName = strings.TrimSpace(cardDoc.Name)
	}
	if result.DisplayName == "" && profileDoc != nil && strings.TrimSpace(profileDoc.Name) != "" {
		result.DisplayName = strings.TrimSpace(profileDoc.Name)
	}
	if cardDoc != nil && strings.TrimSpace(cardDoc.ProfileURL) != "" {
		result.ProfileURL = strings.TrimSpace(cardDoc.ProfileURL)
	} else if profileFetch.Artifact.OK {
		result.ProfileURL = profileFetch.Artifact.URL
	}

	mismatches := make([]string, 0)
	warnings := make([]string, 0)
	consistentSignals := 0

	if didDoc != nil {
		if cardDoc != nil {
			cardIssues := compareCardWithDID(cardDoc, didDoc, target.Origin, didFetch.Artifact.URL)
			mismatches = append(mismatches, cardIssues...)
			if len(cardIssues) == 0 {
				consistentSignals++
			}
		}

		if webFingerDoc != nil {
			wfMismatches, wfWarnings, wfConsistent := compareWebFingerWithDID(webFingerDoc, didDoc, target.Resource, didFetch.Artifact.URL)
			mismatches = append(mismatches, wfMismatches...)
			warnings = append(warnings, wfWarnings...)
			if wfConsistent {
				consistentSignals++
			}
		}

		if profileDoc != nil {
			profileMismatches, profileWarnings, profileConsistent := compareProfileWithArtifacts(profileDoc, didFetch.Artifact.URL, cardFetch.Artifact.URL, webFingerFetch.Artifact.URL)
			mismatches = append(mismatches, profileMismatches...)
			warnings = append(warnings, profileWarnings...)
			if profileConsistent {
				consistentSignals++
			}
			if result.DisplayName == "" {
				result.DisplayName = profileDoc.Name
			}
		}
	} else {
		if cardDoc != nil && strings.TrimSpace(cardDoc.DIDURL) != "" {
			warnings = append(warnings, "agent-card references did.json but no did document could be resolved")
		}
		if webFingerDoc != nil && findWebFingerLink(webFingerDoc, "self") != "" {
			warnings = append(warnings, "webfinger references did.json but no did document could be resolved")
		}
	}

	result.Proofs = collectProofs(target.Origin, result.ProfileURL, result.CanonicalID, didDoc, webFingerDoc, profileDoc)
	if len(mismatches) > 0 {
		result.Status = StatusMismatch
		result.Mismatches = dedupeStrings(mismatches)
	}
	if len(warnings) > 0 {
		result.Warnings = dedupeStrings(warnings)
	}
	if result.Status == StatusResolved && consistentSignals > 0 {
		result.Status = StatusConsistent
	}

	return result, nil
}

func normalizeInput(raw string) (normalizedInput, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return normalizedInput{}, errors.New("inspect input is required")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return normalizedInput{}, fmt.Errorf("parse inspect input: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return normalizedInput{}, fmt.Errorf("inspect input %q must include a host", raw)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return normalizedInput{}, fmt.Errorf("inspect input %q must use http or https", raw)
	}
	host := strings.ToLower(parsed.Host)
	origin := scheme + "://" + host
	cleanPath := parsed.EscapedPath()
	if cleanPath == "" {
		cleanPath = "/"
	}
	resource := origin + "/"

	target := normalizedInput{
		Raw:          strings.TrimSpace(raw),
		Origin:       origin,
		Resource:     resource,
		DIDURL:       origin + "/.well-known/did.json",
		WebFingerURL: withWebFingerResource(origin+"/.well-known/webfinger", resource),
		AgentCardURL: origin + "/.well-known/agent-card.json",
		ProfileURL:   origin + "/profile/",
	}

	switch cleanPath {
	case "/", "":
	case "/.well-known/did.json":
		target.DIDURL = origin + cleanPath
	case "/.well-known/agent-card.json":
		target.AgentCardURL = origin + cleanPath
	case "/.well-known/webfinger":
		target.WebFingerURL = withWebFingerResource(origin+cleanPath, resource)
	default:
		target.ProfileURL = origin + cleanPath
		if strings.HasSuffix(parsed.Path, "/") {
			target.ProfileURL += "/"
		}
	}

	return target, nil
}

func adoptDIDServices(target normalizedInput, doc *didDocument) normalizedInput {
	if doc == nil {
		return target
	}
	for _, service := range doc.Service {
		switch strings.TrimSpace(service.Type) {
		case "WebFinger":
			if endpoint := strings.TrimSpace(service.ServiceEndpoint); endpoint != "" {
				target.WebFingerURL = withWebFingerResource(endpoint, target.Resource)
			}
		case "AgentCard":
			if endpoint := strings.TrimSpace(service.ServiceEndpoint); endpoint != "" {
				target.AgentCardURL = endpoint
			}
		case "ProfilePage":
			if endpoint := strings.TrimSpace(service.ServiceEndpoint); endpoint != "" {
				target.ProfileURL = endpoint
			}
		}
	}
	return target
}

func fetchArtifact(ctx context.Context, client *http.Client, artifactType, rawURL string) fetchResult {
	result := fetchResult{
		Artifact: Artifact{
			Type: artifactType,
			URL:  rawURL,
		},
	}
	if strings.TrimSpace(rawURL) == "" {
		result.Artifact.Error = "artifact url is empty"
		return result
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		result.Artifact.Error = fmt.Sprintf("build request: %v", err)
		return result
	}
	resp, err := client.Do(req)
	if err != nil {
		result.Artifact.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		result.Artifact.HTTPStatus = resp.StatusCode
		result.Artifact.Error = fmt.Sprintf("read response body: %v", err)
		return result
	}
	result.Artifact.HTTPStatus = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Artifact.Error = fmt.Sprintf("unexpected http status %d", resp.StatusCode)
		return result
	}
	result.Artifact.OK = true
	result.Artifact.ContentHash = digest(body)
	result.Body = body
	return result
}

func parseDID(body []byte) (*didDocument, error) {
	var doc didDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode did.json: %w", err)
	}
	if strings.TrimSpace(doc.ID) == "" {
		return nil, errors.New("did.json missing id")
	}
	return &doc, nil
}

func parseWebFinger(body []byte) (*webFingerDocument, error) {
	var doc webFingerDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode webfinger: %w", err)
	}
	if strings.TrimSpace(doc.Subject) == "" && len(doc.Links) == 0 {
		return nil, errors.New("webfinger missing subject and links")
	}
	return &doc, nil
}

func parseAgentCard(body []byte) (*agentCardDocument, error) {
	var doc agentCardDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode agent-card.json: %w", err)
	}
	if strings.TrimSpace(doc.CanonicalID) == "" && strings.TrimSpace(doc.DIDURL) == "" {
		return nil, errors.New("agent-card missing canonical_id and did_url")
	}
	return &doc, nil
}

func parseProfile(body []byte, origin string) profileDocument {
	content := string(body)
	doc := profileDocument{
		Name:        matchFirstText(h1Pattern, content),
		CanonicalID: matchFirstText(profileIDCodeRegexp, content),
	}

	links := make([]string, 0)
	for _, match := range hrefPattern.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		href := strings.TrimSpace(html.UnescapeString(match[1]))
		if href == "" {
			continue
		}
		if resolved := resolveProfileHref(origin, href); resolved != "" {
			links = append(links, resolved)
		}
	}
	doc.Links = dedupeStrings(links)
	return doc
}

func compareCardWithDID(card *agentCardDocument, did *didDocument, origin, didURL string) []string {
	if card == nil || did == nil {
		return nil
	}
	issues := make([]string, 0)
	if card.CanonicalID != "" && strings.TrimSpace(card.CanonicalID) != strings.TrimSpace(did.ID) {
		issues = append(issues, fmt.Sprintf("agent-card canonical_id %q does not match did id %q", card.CanonicalID, did.ID))
	}
	if card.DIDURL != "" && didURL != "" && !sameURL(card.DIDURL, didURL) {
		issues = append(issues, fmt.Sprintf("agent-card did_url %q does not match resolved did url %q", card.DIDURL, didURL))
	}
	if card.Origin != "" && origin != "" && !sameOrigin(card.Origin, origin) {
		issues = append(issues, fmt.Sprintf("agent-card origin %q does not match normalized origin %q", card.Origin, origin))
	}
	if len(card.VerificationMethods) > 0 && !refsCovered(card.VerificationMethods, stringSet(did.Authentication)) {
		issues = append(issues, "agent-card verification_methods are not a subset of did authentication keys")
	}
	return issues
}

func compareWebFingerWithDID(webfinger *webFingerDocument, did *didDocument, resource, didURL string) ([]string, []string, bool) {
	if webfinger == nil || did == nil {
		return nil, nil, false
	}

	mismatches := make([]string, 0)
	warnings := make([]string, 0)
	selfLink := findWebFingerLink(webfinger, "self")
	consistent := false

	if webfinger.Subject != "" && resource != "" && !sameURL(webfinger.Subject, resource) {
		mismatches = append(mismatches, fmt.Sprintf("webfinger subject %q does not match resource %q", webfinger.Subject, resource))
	}
	if selfLink == "" {
		warnings = append(warnings, "webfinger missing self link to did.json")
	} else if didURL != "" && !sameURL(selfLink, didURL) {
		mismatches = append(mismatches, fmt.Sprintf("webfinger self link %q does not match resolved did url %q", selfLink, didURL))
	} else {
		consistent = true
	}
	if len(webfinger.Aliases) > 0 && !containsTrimmed(webfinger.Aliases, did.ID) {
		mismatches = append(mismatches, fmt.Sprintf("webfinger aliases %v do not reference did id %q", webfinger.Aliases, did.ID))
	}
	return mismatches, warnings, consistent
}

func compareProfileWithArtifacts(profile *profileDocument, didURL, cardURL, webfingerURL string) ([]string, []string, bool) {
	if profile == nil {
		return nil, nil, false
	}

	mismatches := make([]string, 0)
	warnings := make([]string, 0)
	consistentSignals := 0

	profileDIDLinks := matchingArtifactLinks(profile.Links, "/.well-known/did.json")
	if len(profileDIDLinks) == 0 {
		warnings = append(warnings, "profile page missing did.json link")
	} else if didURL != "" && !containsURL(profileDIDLinks, didURL) {
		mismatches = append(mismatches, fmt.Sprintf("profile page references did urls %v instead of %q", profileDIDLinks, didURL))
	} else {
		consistentSignals++
	}

	profileCardLinks := matchingArtifactLinks(profile.Links, "/.well-known/agent-card.json")
	if len(profileCardLinks) > 0 && cardURL != "" && !containsURL(profileCardLinks, cardURL) {
		mismatches = append(mismatches, fmt.Sprintf("profile page references agent-card urls %v instead of %q", profileCardLinks, cardURL))
	}
	if len(profileCardLinks) > 0 && cardURL != "" && containsURL(profileCardLinks, cardURL) {
		consistentSignals++
	}

	profileWebFingerLinks := matchingArtifactLinks(profile.Links, "/.well-known/webfinger")
	if len(profileWebFingerLinks) > 0 && webfingerURL != "" && containsURL(profileWebFingerLinks, webfingerURL) {
		consistentSignals++
	}

	return mismatches, warnings, consistentSignals > 0
}

func collectProofs(origin, profileURL, canonicalID string, did *didDocument, webfinger *webFingerDocument, profile *profileDocument) []Proof {
	dedup := make(map[string]Proof)
	addProof := func(proof Proof) {
		key := proof.Type + "|" + proof.URL + "|" + proof.ObservedValue
		if _, exists := dedup[key]; exists {
			return
		}
		dedup[key] = proof
	}

	if did != nil {
		for _, value := range did.AlsoKnownAs {
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			status := "observed"
			if sameURL(trimmed, origin+"/") || (profileURL != "" && sameURL(trimmed, profileURL)) {
				status = "verified"
			}
			addProof(Proof{
				Type:           "also_known_as",
				URL:            trimmed,
				ObservedValue:  trimmed,
				VerifiedStatus: status,
			})
		}
	}

	if webfinger != nil {
		for _, alias := range webfinger.Aliases {
			trimmed := strings.TrimSpace(alias)
			if trimmed == "" {
				continue
			}
			status := "observed"
			if canonicalID != "" && trimmed == canonicalID {
				status = "verified"
			}
			addProof(Proof{
				Type:           "webfinger_alias",
				URL:            origin,
				ObservedValue:  trimmed,
				VerifiedStatus: status,
			})
		}
	}

	if profile != nil {
		for _, link := range profile.Links {
			if link == "" || strings.HasPrefix(link, origin) {
				continue
			}
			addProof(Proof{
				Type:           "profile_link",
				URL:            link,
				ObservedValue:  link,
				VerifiedStatus: "observed",
			})
		}
	}

	proofs := make([]Proof, 0, len(dedup))
	for _, proof := range dedup {
		proofs = append(proofs, proof)
	}
	sort.Slice(proofs, func(i, j int) bool {
		if proofs[i].Type == proofs[j].Type {
			return proofs[i].ObservedValue < proofs[j].ObservedValue
		}
		return proofs[i].Type < proofs[j].Type
	})
	return proofs
}

func withWebFingerResource(rawURL, resource string) string {
	if rawURL == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	query.Set("resource", resource)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func resolveProfileHref(origin, href string) string {
	if strings.TrimSpace(href) == "" {
		return ""
	}
	base, err := url.Parse(origin + "/")
	if err != nil {
		return strings.TrimSpace(href)
	}
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return strings.TrimSpace(href)
	}
	return base.ResolveReference(ref).String()
}

func sameOrigin(left, right string) bool {
	leftOrigin := extractOrigin(left)
	rightOrigin := extractOrigin(right)
	return leftOrigin != "" && leftOrigin == rightOrigin
}

func extractOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)
}

func sameURL(left, right string) bool {
	return normalizeComparableURL(left) == normalizeComparableURL(right)
}

func normalizeComparableURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimSpace(raw)
	}

	cleanPath := path.Clean(parsed.EscapedPath())
	if cleanPath == "." {
		cleanPath = "/"
	}
	if strings.HasSuffix(parsed.EscapedPath(), "/") && cleanPath != "/" {
		cleanPath += "/"
	}

	query := parsed.Query()
	encodedQuery := query.Encode()

	normalized := strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host) + cleanPath
	if encodedQuery != "" {
		normalized += "?" + encodedQuery
	}
	return normalized
}

func summarizeDID(doc *didDocument) string {
	return fmt.Sprintf("id=%s auth=%d services=%d", strings.TrimSpace(doc.ID), len(doc.Authentication), len(doc.Service))
}

func summarizeWebFinger(doc *webFingerDocument) string {
	return fmt.Sprintf("subject=%s aliases=%d links=%d", strings.TrimSpace(doc.Subject), len(doc.Aliases), len(doc.Links))
}

func summarizeAgentCard(doc *agentCardDocument) string {
	return fmt.Sprintf("canonical_id=%s methods=%d", strings.TrimSpace(doc.CanonicalID), len(doc.VerificationMethods))
}

func summarizeProfile(doc profileDocument) string {
	return fmt.Sprintf("name=%s links=%d", strings.TrimSpace(doc.Name), len(doc.Links))
}

func compactArtifacts(artifacts []Artifact) []Artifact {
	compacted := make([]Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.URL == "" {
			continue
		}
		compacted = append(compacted, artifact)
	}
	return compacted
}

func countSuccessfulArtifacts(artifacts []Artifact) int {
	count := 0
	for _, artifact := range artifacts {
		if artifact.OK {
			count++
		}
	}
	return count
}

func findWebFingerLink(doc *webFingerDocument, rel string) string {
	if doc == nil {
		return ""
	}
	for _, link := range doc.Links {
		if strings.TrimSpace(link.Rel) == rel {
			return strings.TrimSpace(link.Href)
		}
	}
	return ""
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	return set
}

func refsCovered(needles []string, allowed map[string]struct{}) bool {
	for _, needle := range needles {
		if _, ok := allowed[strings.TrimSpace(needle)]; !ok {
			return false
		}
	}
	return true
}

func containsTrimmed(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(want) {
			return true
		}
	}
	return false
}

func containsURL(values []string, want string) bool {
	for _, value := range values {
		if sameURL(value, want) {
			return true
		}
	}
	return false
}

func matchingArtifactLinks(links []string, suffix string) []string {
	matches := make([]string, 0)
	for _, link := range links {
		parsed, err := url.Parse(link)
		if err != nil {
			continue
		}
		if strings.HasSuffix(parsed.Path, suffix) {
			matches = append(matches, link)
		}
	}
	return matches
}

func matchFirstText(pattern *regexp.Regexp, content string) string {
	match := pattern.FindStringSubmatch(content)
	if len(match) < 2 {
		return ""
	}
	value := strings.TrimSpace(html.UnescapeString(stripTags(match[1])))
	return strings.Join(strings.Fields(value), " ")
}

func stripTags(content string) string {
	var b strings.Builder
	inTag := false
	for _, r := range content {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
