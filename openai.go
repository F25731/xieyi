package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type ChatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   Usage        `json:"usage"`
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (a *App) handleModels(w http.ResponseWriter, r *http.Request) {
	if !a.isWrapperAuthed(r) {
		jsonError(w, http.StatusUnauthorized, "invalid wrapper secret")
		return
	}
	now := time.Now().Unix()
	data := []map[string]any{{"id": "video-parse", "object": "model", "created": now, "owned_by": "newapi-video-wrapper"}}
	for _, api := range enabledAPIs(a.store.Get()) {
		data = append(data, map[string]any{"id": "video-parse-" + api.ID, "object": "model", "created": now, "owned_by": "newapi-video-wrapper"})
	}
	jsonOK(w, map[string]any{"object": "list", "data": data})
}

func (a *App) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.isWrapperAuthed(r) {
		jsonError(w, http.StatusUnauthorized, "invalid wrapper secret")
		return
	}
	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	input := lastUserContent(req.Messages)
	if input == "" {
		jsonError(w, http.StatusBadRequest, "missing user message")
		return
	}
	cfg := a.store.Get()
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs+5000)*time.Millisecond)
	defer cancel()
	result := a.dispatcher.Submit(ctx, input, modelToAPIID(req.Model))
	contentBytes, _ := json.MarshalIndent(result, "", "  ")
	content := string(contentBytes)
	resp := ChatCompletionResponse{
		ID:      "chatcmpl-" + randomToken(12),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   first(req.Model, "video-parse"),
		Choices: []ChatChoice{{
			Index: 0,
			Message: ChatMessage{
				Role:    "assistant",
				Content: content,
			},
			FinishReason: "stop",
		}},
		Usage: Usage{
			PromptTokens:     approxTokens(input),
			CompletionTokens: approxTokens(content),
			TotalTokens:      approxTokens(input) + approxTokens(content),
		},
	}
	jsonOK(w, resp)
}

func (a *App) handleParse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !a.isWrapperAuthed(r) {
		jsonError(w, http.StatusUnauthorized, "invalid wrapper secret")
		return
	}
	var body struct {
		URL   string `json:"url"`
		APIID string `json:"apiId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg := a.store.Get()
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs+5000)*time.Millisecond)
	defer cancel()
	result := a.dispatcher.Submit(ctx, body.URL, body.APIID)
	jsonOK(w, result)
}

func lastUserContent(messages []ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && strings.TrimSpace(messages[i].Content) != "" {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	if len(messages) > 0 {
		return strings.TrimSpace(messages[len(messages)-1].Content)
	}
	return ""
}

func modelToAPIID(model string) string {
	model = strings.TrimSpace(model)
	switch model {
	case "", "video-parse", "media-parser", "parse":
		return ""
	}
	return strings.TrimPrefix(model, "video-parse-")
}

func approxTokens(s string) int {
	n := len([]rune(s)) / 2
	if n < 1 {
		return 1
	}
	return n
}
