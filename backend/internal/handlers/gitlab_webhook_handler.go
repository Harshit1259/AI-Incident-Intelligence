package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// GitLabWebhookHandler processes GitLab webhook events.
type GitLabWebhookHandler struct {
	changeLinker *services.ChangeLinkerService
	webhookToken string
}

func NewGitLabWebhookHandler(cl *services.ChangeLinkerService, token string) *GitLabWebhookHandler {
	return &GitLabWebhookHandler{changeLinker: cl, webhookToken: token}
}

func (h *GitLabWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Enforce body size limit before reading.
	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		api.WriteError(w, http.StatusRequestEntityTooLarge, "request body too large (max 1 MiB)")
		return
	}
	defer r.Body.Close()

	// Verify token if configured — constant-time comparison prevents timing attacks.
	// GitLab sends a plain shared secret (not HMAC) per their webhook specification.
	if h.webhookToken != "" {
		token := r.Header.Get("X-Gitlab-Token")
		if subtle.ConstantTimeCompare([]byte(token), []byte(h.webhookToken)) != 1 {
			api.WriteError(w, http.StatusUnauthorized, "invalid token")
			return
		}
	}

	eventType := r.Header.Get("X-Gitlab-Event")
	if eventType == "" {
		api.WriteError(w, http.StatusBadRequest, "missing X-Gitlab-Event header")
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	ce, ok := extractGitLabChange(eventType, payload)
	if !ok {
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	ce.ChangeSource = "gitlab"
	ce.TenantID = resolveWebhookTenant(r)

	incident := h.changeLinker.LinkDeployEnriched(ce)
	if incident != nil {
		slog.InfoContext(r.Context(), "gitlab_webhook: linked event to incident", "event_type", eventType, "service", ce.Service, "incident_id", incident.ID)
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "processed"})
}

// extractGitLabChange parses a GitLab webhook payload into an enriched ChangeEvent.
func extractGitLabChange(eventType string, payload map[string]any) (models.ChangeEvent, bool) {
	svcName := ""
	if repo, ok := payload["repository"].(map[string]any); ok {
		if name, ok := repo["name"].(string); ok {
			svcName = normalizeServiceName(name)
		}
	}
	if svcName == "" {
		if project, ok := payload["project"].(map[string]any); ok {
			if name, ok := project["name"].(string); ok {
				svcName = normalizeServiceName(name)
			}
		}
	}
	if svcName == "" {
		return models.ChangeEvent{}, false
	}

	ce := models.ChangeEvent{
		Service:     svcName,
		Timestamp:   time.Now(),
		Environment: "production",
		Metadata:    map[string]string{},
	}

	switch eventType {
	case "Push Hook":
		ce.Type = "commit"
		if after, ok := payload["after"].(string); ok {
			ce.CommitSHA = after
			if len(after) >= 8 {
				ce.Version = after[:8]
			}
		}
		if commits, ok := payload["commits"].([]any); ok {
			ce.ChangedFilesCount = len(commits)
			if len(commits) > 0 {
				if c, ok := commits[0].(map[string]any); ok {
					if msg, ok := c["message"].(string); ok {
						ce.Description = msg
					}
					if author, ok := c["author"].(map[string]any); ok {
						if name, ok := author["name"].(string); ok {
							ce.Author = name
						}
					}
				}
			}
		}
		if name, ok := payload["user_name"].(string); ok && ce.Author == "" {
			ce.Author = name
		}

	case "Tag Push Hook":
		ce.Type = "release"
		if ref, ok := payload["ref"].(string); ok {
			ce.Version = strings.TrimPrefix(ref, "refs/tags/")
		}
		ce.Description = "Tag push"
		if name, ok := payload["user_name"].(string); ok {
			ce.Author = name
		}

	case "Deployment Hook":
		ce.Type = "deployment"
		if ref, ok := payload["ref"].(string); ok {
			ce.Version = ref
		} else if depID, ok := payload["deployment_id"].(float64); ok {
			ce.Version = fmt.Sprintf("%d", int64(depID))
		}
		if env, ok := payload["environment"].(string); ok {
			ce.Environment = env
			ce.Description = env
		}
		if name, ok := payload["user"].(map[string]any); ok {
			if login, ok := name["username"].(string); ok {
				ce.Author = login
			}
		}

	case "Release Hook":
		ce.Type = "release"
		if tag, ok := payload["tag"].(string); ok {
			ce.Version = tag
		}
		if name, ok := payload["name"].(string); ok {
			ce.Description = name
		}

	default:
		return models.ChangeEvent{}, false
	}

	return ce, true
}
