package challenge

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"kubefix-labs/backend/internal/auth"
	"kubefix-labs/backend/internal/db"
	"kubefix-labs/backend/internal/docker"
)

// verification defines how a solution is automatically checked.
type verification struct {
	Type         string   `json:"type"` // command | file | script
	ContainerCmd []string `json:"container_cmd,omitempty"`
	Expected     string   `json:"expected,omitempty"`
	Match        string   `json:"match,omitempty"` // contains | equals | exit_zero
	Script       string   `json:"script,omitempty"`
}

// Service orchestrates challenge lifecycle: list, start, verify, terminal.
type Service struct {
	store  *db.Store
	docker docker.Client
}

// NewService creates a challenge service.
func NewService(store *db.Store, d docker.Client) *Service {
	return &Service{store: store, docker: d}
}

// HandleListChallenges returns published challenges, optionally filtered.
func (s *Service) HandleListChallenges(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	difficulty := r.URL.Query().Get("difficulty")

	chals, err := s.store.ListChallenges(r.Context(), category, difficulty)
	if err != nil {
		http.Error(w, "failed to list challenges", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"challenges": chals})
}

// HandleGetChallenge returns a single published challenge by ID.
func (s *Service) HandleGetChallenge(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid challenge id", http.StatusBadRequest)
		return
	}

	c, err := s.store.GetChallenge(r.Context(), id, true)
	if err != nil {
		http.Error(w, "challenge not found", http.StatusNotFound)
		return
	}

	c.VerificationJSON = ""
	c.ExpectedAnswer = ""
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"challenge": c})
}

// HandleStartChallenge allocates an isolated environment and consumes one credit atomically.
// Refreshing/reconnecting to an existing active attempt does NOT consume another credit.
func (s *Service) HandleStartChallenge(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	idStr := r.PathValue("id")
	challengeID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid challenge id", http.StatusBadRequest)
		return
	}

	challenge, err := s.store.GetChallenge(r.Context(), challengeID, true)
	if err != nil {
		http.Error(w, "challenge not found", http.StatusNotFound)
		return
	}
	if !challenge.IsPublished {
		http.Error(w, "challenge not published", http.StatusForbidden)
		return
	}

	tx, err := s.store.Pool().Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to begin transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	// Check for existing active attempt (reconnect does not consume credit).
	existing, err := s.store.GetActiveAttempt(r.Context(), userID, challengeID)
	if err == nil && existing != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"attempt": existing,
			"resumed": true,
			"message": "reconnected to existing active attempt",
		})
		return
	}

	// Atomically consume one credit; fail if balance insufficient.
	var newBalance int
	err = tx.QueryRow(r.Context(), `
		UPDATE users
		SET credits = credits - 1, updated_at = now()
		WHERE id = $1 AND credits >= 1
		RETURNING credits
	`, userID).Scan(&newBalance)
	if err != nil {
		// Insufficient credits or other DB error.
		http.Error(w, "insufficient challenge credits", http.StatusPaymentRequired)
		return
	}

	// Allocate environment (best-effort Docker). A failed allocation refunds the credit.
	envID, envType, allocErr := s.allocateEnvironment(r.Context(), challenge)
	if allocErr != nil {
		// Refund the credit since allocation failed.
		if _, rerr := s.store.AdjustCredits(r.Context(), tx, userID, 1, db.ReasonRefund, ""); rerr != nil {
			log.Printf("failed to refund credit after allocation failure: %v", rerr)
		}
		_ = tx.Commit(r.Context())
		http.Error(w, "infrastructure allocation failed: "+allocErr.Error(), http.StatusBadGateway)
		return
	}

	attempt, err := s.store.CreateAttempt(r.Context(), tx, userID, challengeID, envID, envType, 1)
	if err != nil {
		// Refund if attempt record creation failed.
		if _, rerr := s.store.AdjustCredits(r.Context(), tx, userID, 1, db.ReasonRefund, ""); rerr != nil {
			log.Printf("failed to refund credit: %v", rerr)
		}
		_ = tx.Commit(r.Context())
		http.Error(w, "failed to create attempt record", http.StatusInternalServerError)
		return
	}

	// Record credit consumption event.
	if _, err := s.store.AdjustCredits(r.Context(), tx, userID, -1, db.ReasonChallengeStart, attempt.ID.String()); err != nil {
		log.Printf("failed to record credit event: %v", err)
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to commit attempt", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"attempt":        attempt,
		"credits_left":   newBalance,
		"environment_id": envID,
		"message":        "challenge environment allocated",
	})
}

// allocateEnvironment starts an isolated challenge environment.
func (s *Service) allocateEnvironment(ctx context.Context, c *db.Challenge) (string, string, error) {
	switch c.Category {
	case db.CategoryDocker:
		return s.allocateDocker(ctx, c)
	case db.CategoryK8s:
		return s.allocateKubernetes(ctx, c)
	default:
		return "", "", errors.New("unknown challenge category")
	}
}

func (s *Service) allocateDocker(ctx context.Context, c *db.Challenge) (string, string, error) {
	if s.docker == nil || !s.docker.Available() {
		return "", "", errors.New("docker daemon unavailable")
	}

	image := "ubuntu:22.04" // default; challenge definitions can override via metadata
	name := c.Title
	env := []string{"CHALLENGE_ID=" + c.ID.String()}
	containerID, cleanup, err := s.docker.AllocateContainer(ctx, image, name, env, []string{"sleep", "infinity"})
	if err != nil {
		return "", "", err
	}
	_ = cleanup // cleanup is invoked on attempt termination elsewhere

	return containerID, db.EnvDocker, nil
}

func (s *Service) allocateKubernetes(ctx context.Context, c *db.Challenge) (string, string, error) {
	// Placeholder: dedicated namespace / disposable cluster. Without a cluster API
	// available, return a descriptive error so the credit is refunded.
	return "", "", errors.New("kubernetes cluster integration not configured in this build")
}

// HandleVerifyChallenge checks the submitted solution against the challenge's verification.
func (s *Service) HandleVerifyChallenge(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	idStr := r.PathValue("id")
	challengeID, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid challenge id", http.StatusBadRequest)
		return
	}

	var body struct {
		Answer string `json:"answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Find active attempt for this user/challenge.
	attempt, err := s.store.GetActiveAttempt(r.Context(), userID, challengeID)
	if err != nil {
		http.Error(w, "no active attempt found", http.StatusNotFound)
		return
	}

	challenge, err := s.store.GetChallenge(r.Context(), challengeID, true)
	if err != nil {
		http.Error(w, "challenge not found", http.StatusNotFound)
		return
	}

	passed, feedback := s.verifySolution(r.Context(), challenge, attempt, body.Answer)

	now := time.Now()
	solveTime := int(now.Sub(attempt.StartedAt).Seconds())

	tx, err := s.store.Pool().Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to begin transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	status := db.AttemptFailed
	if passed {
		status = db.AttemptPassed
	}

	if err := s.store.CompleteAttempt(r.Context(), tx, attempt.ID, status, now, solveTime); err != nil {
		http.Error(w, "failed to complete attempt", http.StatusInternalServerError)
		return
	}

	if _, err := s.store.CreateSubmission(r.Context(), tx, attempt.ID, body.Answer, passed, feedback); err != nil {
		http.Error(w, "failed to record submission", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to commit verification", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"passed":             passed,
		"feedback":           feedback,
		"attempt":            attempt.ID,
		"solve_time_seconds": solveTime,
	})
}

// verifySolution evaluates the submitted answer. A failed check does NOT consume a credit.
func (s *Service) verifySolution(ctx context.Context, c *db.Challenge, a *db.Attempt, answer string) (bool, string) {
	var v verification
	_ = json.Unmarshal([]byte(c.VerificationJSON), &v)

	if v.Type == "" {
		// No automated verification configured; fall back to manual answer match.
		if strings.TrimSpace(answer) == strings.TrimSpace(c.ExpectedAnswer) {
			return true, "Solution accepted."
		}
		return false, "Incorrect. Try again."
	}

	switch v.Type {
	case "command":
		return s.verifyByCommand(ctx, c, a, v)
	case "exit_zero":
		return s.verifyByExitZero(ctx, c, a, v)
	default:
		return false, "Verification method not implemented."
	}
}

func (s *Service) verifyByCommand(ctx context.Context, c *db.Challenge, a *db.Attempt, v verification) (bool, string) {
	if s.docker == nil || !s.docker.Available() || a.EnvironmentID == "" {
		return false, "Cannot verify: environment unavailable."
	}

	output, err := s.docker.Exec(ctx, a.EnvironmentID, v.ContainerCmd)
	if err != nil {
		return false, "Verification command failed: " + err.Error()
	}

	switch v.Match {
	case "equals":
		if strings.TrimSpace(output) == v.Expected {
			return true, "Solution verified."
		}
		return false, "Output did not match expected value."
	default: // contains
		if strings.Contains(output, v.Expected) {
			return true, "Solution verified."
		}
		return false, "Expected output not found."
	}
}

func (s *Service) verifyByExitZero(ctx context.Context, c *db.Challenge, a *db.Attempt, v verification) (bool, string) {
	_, err := s.docker.Exec(ctx, a.EnvironmentID, v.ContainerCmd)
	if err != nil {
		return false, "Verification command returned non-zero exit code."
	}
	return true, "Solution verified."
}

// HandleMyAttempts returns the current user's recent attempts.
func (s *Service) HandleMyAttempts(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	attempts, err := s.store.ListMyAttempts(r.Context(), userID, 50)
	if err != nil {
		http.Error(w, "failed to list attempts", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"attempts": attempts})
}

// HandleTerminal proxies a terminal to a challenge environment (best-effort).
func (s *Service) HandleTerminal(w http.ResponseWriter, r *http.Request) {
	envID := r.PathValue("environment_id")
	if envID == "" {
		http.Error(w, "missing environment id", http.StatusBadRequest)
		return
	}
	if s.docker == nil || !s.docker.Available() {
		http.Error(w, "terminal backend unavailable", http.StatusServiceUnavailable)
		return
	}
	// In a full implementation this would upgrade the connection to a websocket pty.
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("terminal endpoint for " + envID + " (websocket pty not implemented in this build)"))
}

// Admin handlers below allow creating/editing/publishing/deleting challenges.

// HandleListAdminChallenges lists all challenges for the admin dashboard.
func (s *Service) HandleListAdminChallenges(w http.ResponseWriter, r *http.Request) {
	chals, err := s.store.ListAdminChallenges(r.Context())
	if err != nil {
		http.Error(w, "failed to list challenges", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"challenges": chals})
}

// HandleGetAdminChallenge returns a single challenge (admin, no publish filter).
func (s *Service) HandleGetAdminChallenge(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid challenge id", http.StatusBadRequest)
		return
	}
	c, err := s.store.GetChallenge(r.Context(), id, false)
	if err != nil {
		http.Error(w, "challenge not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"challenge": c})
}

// HandleCreateChallenge inserts a new challenge (admin only).
func (s *Service) HandleCreateChallenge(w http.ResponseWriter, r *http.Request) {
	var c db.Challenge
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if c.Title == "" || c.Description == "" || c.Category == "" || c.Difficulty == "" {
		http.Error(w, "title, description, category, and difficulty are required", http.StatusBadRequest)
		return
	}
	created, err := s.store.CreateChallenge(r.Context(), c)
	if err != nil {
		http.Error(w, "failed to create challenge", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"challenge": created})
}

// HandleUpdateChallenge modifies an existing challenge (admin only).
func (s *Service) HandleUpdateChallenge(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid challenge id", http.StatusBadRequest)
		return
	}
	var c db.Challenge
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	updated, err := s.store.UpdateChallenge(r.Context(), id, c)
	if err != nil {
		http.Error(w, "failed to update challenge", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"challenge": updated})
}

// HandlePublishChallenge toggles the published flag (admin only).
func (s *Service) HandlePublishChallenge(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid challenge id", http.StatusBadRequest)
		return
	}
	var body struct {
		Published bool `json:"published"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.store.PublishChallenge(r.Context(), id, body.Published); err != nil {
		http.Error(w, "failed to update publish state", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "is_published": body.Published})
}

// HandleDeleteChallenge removes a challenge (admin only).
func (s *Service) HandleDeleteChallenge(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid challenge id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteChallenge(r.Context(), id); err != nil {
		http.Error(w, "failed to delete challenge", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "deleted"})
}
