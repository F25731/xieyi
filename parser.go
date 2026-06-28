package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type PublicAPI struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Group  string `json:"group"`
	Method string `json:"method"`
}

type InputInfo struct {
	OriginalURL   string   `json:"originalUrl"`
	NormalizedURL string   `json:"normalizedUrl"`
	Extracted     bool     `json:"extracted"`
	Candidates    []string `json:"candidates"`
	AutoDetected  bool     `json:"autoDetected"`
}

type ParseResult struct {
	OK         bool             `json:"ok"`
	Status     int              `json:"status"`
	API        *PublicAPI       `json:"api,omitempty"`
	Input      InputInfo        `json:"input"`
	Upstream   any              `json:"upstream,omitempty"`
	Normalized NormalizedResult `json:"normalized,omitempty"`
	Error      string           `json:"error,omitempty"`
	DurationMs int64            `json:"durationMs"`
}

type MediaItem struct {
	Label    string `json:"label"`
	URL      string `json:"url"`
	Type     string `json:"type"`
	Details  string `json:"details,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type NormalizedResult struct {
	Title  string      `json:"title"`
	Author string      `json:"author,omitempty"`
	Avatar string      `json:"avatar,omitempty"`
	Cover  string      `json:"cover,omitempty"`
	Videos []MediaItem `json:"videos"`
	Images []MediaItem `json:"images"`
	Audios []MediaItem `json:"audios"`
	Links  []MediaItem `json:"links"`
}

type normalizedShare struct {
	URL        string
	Extracted  bool
	Candidates []string
}

func parseMedia(ctx context.Context, cfg Config, stats *Stats, input, apiID string) ParseResult {
	start := time.Now()
	result := ParseResult{Input: InputInfo{OriginalURL: input, AutoDetected: apiID == ""}}
	api, ok := resolveAPI(cfg, apiID, input)
	if !ok {
		result.Status = 400
		if apiID != "" {
			result.Status = 404
			result.Error = "requested platform is disabled or not found"
		} else {
			result.Error = "cannot detect platform from input"
		}
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}
	normalized := normalizeShareText(input, api)
	result.API = &PublicAPI{ID: api.ID, Name: api.Name, Group: api.Group, Method: api.Method}
	result.Input = InputInfo{
		OriginalURL:   input,
		NormalizedURL: normalized.URL,
		Extracted:     normalized.Extracted,
		Candidates:    normalized.Candidates,
		AutoDetected:  apiID == "",
	}
	if normalized.URL == "" || api.EndpointURL == "" {
		result.Status = 400
		result.Error = "empty input url or upstream endpoint"
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}
	upstream, status, err := callUpstream(ctx, cfg, api, normalized.URL, stats)
	result.Status = status
	result.Upstream = upstream
	if err != nil {
		result.Error = err.Error()
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}
	result.OK = status >= 200 && status < 300
	if !result.OK {
		result.Error = fmt.Sprintf("upstream returned status %d", status)
	}
	result.Normalized = normalizeResult(upstream)
	result.DurationMs = time.Since(start).Milliseconds()
	return result
}

func resolveAPI(cfg Config, requested, input string) (APIConfig, bool) {
	apis := enabledAPIs(cfg)
	if requested != "" {
		requested = strings.TrimPrefix(requested, "video-parse-")
		for _, api := range apis {
			if api.ID == requested {
				return api, true
			}
		}
		return APIConfig{}, false
	}
	for _, u := range extractURLs(input) {
		for _, api := range apis {
			for _, domain := range api.Domains {
				if hostMatches(u, domain) {
					return api, true
				}
			}
		}
	}
	return APIConfig{}, false
}

func normalizeShareText(text string, api APIConfig) normalizedShare {
	raw := strings.TrimSpace(text)
	urls := extractURLs(raw)
	if len(urls) == 0 {
		return normalizedShare{URL: raw}
	}
	for _, candidate := range urls {
		for _, domain := range api.Domains {
			if hostMatches(candidate, domain) {
				return normalizedShare{URL: candidate, Extracted: true, Candidates: urls}
			}
		}
	}
	return normalizedShare{URL: urls[0], Extracted: true, Candidates: urls}
}

func callUpstream(ctx context.Context, cfg Config, api APIConfig, shareURL string, stats *Stats) (any, int, error) {
	client := &http.Client{Timeout: time.Duration(cfg.RequestTimeoutMs) * time.Millisecond}
	key := first(api.APIKey, cfg.GlobalAPIKey)
	var lastErr error
	var lastStatus int
	for attempt := 0; attempt <= cfg.RetryTimes; attempt++ {
		req, err := buildUpstreamRequest(ctx, api, shareURL, key)
		if err != nil {
			return nil, 0, err
		}
		atomic.AddUint64(&stats.UpstreamRequests, 1)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
		_ = resp.Body.Close()
		lastStatus = resp.StatusCode
		if readErr != nil {
			lastErr = readErr
			continue
		}
		parsed := parseUpstreamBody(body)
		if resp.StatusCode >= 500 && attempt < cfg.RetryTimes {
			lastErr = fmt.Errorf("upstream status %d", resp.StatusCode)
			time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			continue
		}
		return parsed, resp.StatusCode, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("upstream status %d", lastStatus)
	}
	return nil, lastStatus, lastErr
}

func buildUpstreamRequest(ctx context.Context, api APIConfig, shareURL, key string) (*http.Request, error) {
	params := url.Values{}
	params.Set(api.URLParam, shareURL)
	if key != "" {
		params.Set(api.KeyParam, key)
	}
	if api.Method == "POST" {
		req, err := http.NewRequestWithContext(ctx, "POST", api.EndpointURL, strings.NewReader(params.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("content-type", "application/x-www-form-urlencoded; charset=utf-8")
		req.Header.Set("user-agent", "newapi-video-wrapper/1.0")
		return req, nil
	}
	u, err := url.Parse(api.EndpointURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for k, vals := range params {
		for _, v := range vals {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("user-agent", "newapi-video-wrapper/1.0")
	return req, nil
}

func parseUpstreamBody(body []byte) any {
	var v any
	if err := json.Unmarshal(body, &v); err == nil {
		return v
	}
	return map[string]any{"raw": string(body)}
}

var urlRe = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `，。！？、；：）】》]+`)

func extractURLs(text string) []string {
	matches := urlRe.FindAllString(text, -1)
	out := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, m := range matches {
		clean := cleanURL(m)
		if clean != "" && !seen[clean] {
			seen[clean] = true
			out = append(out, clean)
		}
	}
	return out
}

func cleanURL(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), ")]}>,.?!;:'\"。，、；：！？）】》")
}

func hostMatches(raw, domain string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	d := strings.TrimPrefix(strings.ToLower(domain), "www.")
	return host == d || strings.HasSuffix(host, "."+d) || strings.Contains(host, d)
}

func normalizeResult(upstream any) NormalizedResult {
	source := unwrapData(upstream)
	urls := collectURLs(source, "")
	seen := map[string]bool{}
	videos := make([]MediaItem, 0)
	images := make([]MediaItem, 0)
	audios := make([]MediaItem, 0)
	links := make([]MediaItem, 0)
	for _, item := range urls {
		if seen[item.URL] {
			continue
		}
		seen[item.URL] = true
		item.Type = classifyURL(item.Label, item.URL)
		item.Filename = buildFilename(item.Type, item.Label, len(seen))
		switch item.Type {
		case "video":
			videos = append(videos, item)
		case "image":
			images = append(images, item)
		case "audio":
			audios = append(audios, item)
		default:
			links = append(links, item)
		}
	}
	sort.SliceStable(videos, func(i, j int) bool { return videos[i].Label < videos[j].Label })
	return NormalizedResult{
		Title:  first(pickString(source, "title", "desc", "description", "text"), "解析结果"),
		Author: pickString(source, "author", "nickname", "name", "user"),
		Avatar: pickString(source, "avatar", "avatar_url", "head", "headimg"),
		Cover:  first(pickString(source, "cover", "poster", "thumbnail", "thumb", "image"), firstURL(images)),
		Videos: videos,
		Images: images,
		Audios: audios,
		Links:  links,
	}
}

func unwrapData(v any) any {
	if m, ok := v.(map[string]any); ok {
		if data, ok := m["data"]; ok {
			return data
		}
	}
	return v
}

func collectURLs(v any, path string) []MediaItem {
	var out []MediaItem
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(strings.ToLower(x), "http://") || strings.HasPrefix(strings.ToLower(x), "https://") {
			out = append(out, MediaItem{Label: first(path, "url"), URL: x})
		}
	case []any:
		for i, item := range x {
			label := strings.TrimSpace(path + " " + strconv.Itoa(i+1))
			out = append(out, collectURLs(item, label)...)
		}
	case map[string]any:
		for k, item := range x {
			label := k
			if path != "" {
				label = path + " " + k
			}
			out = append(out, collectURLs(item, label)...)
		}
	}
	return out
}

func classifyURL(label, raw string) string {
	text := strings.ToLower(label + " " + raw)
	ext := strings.ToLower(filepath.Ext(strings.Split(raw, "?")[0]))
	mt := mime.TypeByExtension(ext)
	switch {
	case strings.Contains(mt, "video/") || regexp.MustCompile(`(?i)(video|play|mp4|m3u8|mov|webm|download|url)`).MatchString(text):
		return "video"
	case regexp.MustCompile(`(?i)(live[_ -]?photo|video_mp4|mime_type=video)`).MatchString(text):
		return "video"
	case strings.Contains(mt, "audio/") || regexp.MustCompile(`(?i)(music|audio|mp3|m4a|wav|sound)`).MatchString(text):
		return "audio"
	case strings.Contains(mt, "image/") || regexp.MustCompile(`(?i)(cover|poster|thumb|avatar|image|img|pic|photo|logo)`).MatchString(text):
		return "image"
	default:
		return "link"
	}
}

func pickString(v any, keys ...string) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range keys {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func firstURL(items []MediaItem) string {
	if len(items) == 0 {
		return ""
	}
	return items[0].URL
}

func buildFilename(kind, label string, index int) string {
	name := regexp.MustCompile(`[^\w\p{Han}-]+`).ReplaceAllString(label, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		name = "media"
	}
	if len([]rune(name)) > 60 {
		name = string([]rune(name)[:60])
	}
	return fmt.Sprintf("%s_%d_%s", kind, index, name)
}
