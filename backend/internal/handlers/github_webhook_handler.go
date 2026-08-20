package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// GitHubWebhookHandler processes GitHub webhook events and links them to incidents.
type GitHubWebhookHandler struct {
	changeLinker  *services.ChangeLinkerService
	webhookSecret string
}

func NewGitHubWebhookHandler(cl *services.ChangeLinkerService, secret string) *GitHubWebhookHandler {
	return &GitHubWebhookHandler{changeLinker: cl, webhookSecret: secret}
}

func (h *GitHubWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	// Verify HMAC signature if secret is configured.
	if h.webhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifyGitHubSignature(h.webhookSecret, sig, body) {
			api.WriteError(w, http.StatusUnauthorized, "invalid signature")
			return
		}
	}

	event := r.Header.Get("X-GitHub-Event")
	if event == "" {
		api.WriteError(w, http.StatusBadRequest, "missing X-GitHub-Event header")
		return
	}

	// Parse the payload as a generic map.
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	ce, ok := extractGitHubChange(event, payload)
	if !ok {
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}
	ce.ChangeSource = "github"
	ce.TenantID = resolveWebhookTenant(r)

	incident := h.changeLinker.LinkDeployEnriched(ce)
	if incident != nil {
		slog.InfoContext(r.Context(), "github_webhook: linked event to incident", "event", event, "service", ce.Service, "incident_id", incident.ID)
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "processed"})
}

// extractGitHubChange parses a GitHub webhook payload into an enriched ChangeEvent.
func extractGitHubChange(event string, payload map[string]any) (models.ChangeEvent, bool) {
	repoName := extractRepoName(payload)
	if repoName == "" {
		return models.ChangeEvent{}, false
	}

	ce := models.ChangeEvent{
		Service:     normalizeServiceName(repoName),
		Timestamp:   time.Now(),
		Environment: "production",
		Metadata:    map[string]string{},
	}

	switch event {
	case "push":
		ce.Type = "commit"
		if headCommit, ok := payload["head_commit"].(map[string]any); ok {
			if sha, ok := headCommit["id"].(string); ok {
				ce.CommitSHA = sha
				if len(sha) >= 8 {
					ce.Version = sha[:8]
				}
			}
			if msg, ok := headCommit["message"].(string); ok {
				ce.Description = msg
			}
			if author, ok := headCommit["author"].(map[string]any); ok {
				if name, ok := author["name"].(string); ok {
					ce.Author = name
				}
			}
		}
		if commits, ok := payload["commits"].([]any); ok {
			ce.ChangedFilesCount = len(commits)
		}
		if ref, ok := payload["ref"].(string); ok {
			if branch, found := strings.CutPrefix(ref, "refs/heads/"); found {
				ce.Metadata["branch"] = branch
			}
		}

	case "deployment":
		ce.Type = "deployment"
		if dep, ok := payload["deployment"].(map[string]any); ok {
			if ref, ok := dep["ref"].(string); ok {
				ce.Version = ref
			}
			if sha, ok := dep["sha"].(string); ok {
				ce.CommitSHA = sha
			}
			if task, ok := dep["task"].(string); ok {
				ce.Description = task
			}
			if env, ok := dep["environment"].(string); ok && env != "" {
				ce.Environment = env
			}
			if creator, ok := dep["creator"].(map[string]any); ok {
				if login, ok := creator["login"].(string); ok {
					ce.Author = login
				}
			}
		}

	case "release":
		ce.Type = "release"
		if rel, ok := payload["release"].(map[string]any); ok {
			if tag, ok := rel["tag_name"].(string); ok {
				ce.Version = tag
			}
			if name, ok := rel["name"].(string); ok {
				ce.Description = name
			}
			if author, ok := rel["author"].(map[string]any); ok {
				if login, ok := author["login"].(string); ok {
					ce.Author = login
				}
			}
		}

	case "deployment_status":
		return models.ChangeEvent{}, false

	default:
		return models.ChangeEvent{}, false
	}

	return ce, true
}

// extractRepoName gets the repository name from the payload (after "/").
func extractRepoName(payload map[string]any) string {
	repo, ok := payload["repository"].(map[string]any)
	if !ok {
		return ""
	}
	fullName, _ := repo["full_name"].(string)
	if fullName == "" {
		name, _ := repo["name"].(string)
		return name
	}
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return fullName
}

// normalizeServiceName strips common suffixes from a repo name.
func normalizeServiceName(name string) string {
	lower := strings.ToLower(name)
	for _, suffix := range []string{"-service", "-api", "-svc"} {
		if strings.HasSuffix(lower, suffix) {
			trimmed := lower[:len(lower)-len(suffix)]
			if len(trimmed) > 0 {
				return trimmed
			}
		}
	}
	return lower
}

// resolveWebhookTenant extracts the tenant ID for unauthenticated SCM webhook calls.
// Priority: JWT claims → X-Source-Token header (format: "t_{tenantID}_{rand}") → "default".
func resolveWebhookTenant(r *http.Request) string {
	if t, ok := middleware.TenantFromRequestStrict(r); ok {
		return t
	}
	token := r.Header.Get("X-Source-Token")
	if token == "" {
		return "default"
	}
	parts := strings.SplitN(token, "_", 3)
	if len(parts) >= 2 && parts[0] == "t" {
		return parts[1]
	}
	return "default"
}

// verifyGitHubSignature checks the X-Hub-Signature-256 header.
func verifyGitHubSignature(secret, signature string, body []byte) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	sig := strings.TrimPrefix(signature, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sig))
}
