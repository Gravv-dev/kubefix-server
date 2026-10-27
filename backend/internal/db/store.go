package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	RoleUser  = "user"
	RoleAdmin = "admin"

	CategoryDocker = "docker"
	CategoryK8s    = "kubernetes"

	DifficultyEasy   = "easy"
	DifficultyMedium = "medium"
	DifficultyHard   = "hard"

	AttemptInProgress = "in_progress"
	AttemptPassed     = "passed"
	AttemptFailed     = "failed"
	AttemptExpired    = "expired"

	EnvDocker = "docker"
	EnvK8s    = "kubernetes"

	ReasonChallengeStart = "challenge_start"
	ReasonRestart        = "restart"
	ReasonAdminAdjust    = "admin_adjust"
	ReasonRefund         = "refund"
)

// User represents an authenticated account.
type User struct {
	ID        uuid.UUID `json:"id"`
	GoogleSub string    `json:"google_sub"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Picture   string    `json:"picture"`
	Role      string    `json:"role"`
	Credits   int       `json:"credits"`
	CreatedAt time.Time `json:"created_at"`
}

// Challenge represents a troubleshooting challenge.
type Challenge struct {
	ID               uuid.UUID `json:"id"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Category         string    `json:"category"`
	Difficulty       string    `json:"difficulty"`
	Instructions     string    `json:"instructions"`
	Hints            []string  `json:"hints"`
	Points           int       `json:"points"`
	IsPublished      bool      `json:"is_published"`
	VerificationJSON string    `json:"verification_json,omitempty"`
	ExpectedAnswer   string    `json:"expected_answer,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// Attempt represents a user's attempt at a challenge.
type Attempt struct {
	ID               uuid.UUID `json:"id"`
	UserID           uuid.UUID `json:"user_id"`
	ChallengeID      uuid.UUID `json:"challenge_id"`
	Status           string    `json:"status"`
	EnvironmentID    string    `json:"environment_id"`
	EnvironmentType  string    `json:"environment_type"`
	CreditsConsumed  int       `json:"credits_consumed"`
	StartedAt        time.Time `json:"started_at"`
	EndedAt          time.Time `json:"ended_at"`
	SolvedAt         time.Time `json:"solved_at"`
	SolveTimeSeconds int       `json:"solve_time_seconds"`
}

// Submission represents a solution check.
type Submission struct {
	ID        uuid.UUID `json:"id"`
	AttemptID uuid.UUID `json:"attempt_id"`
	Answer    string    `json:"answer"`
	Passed    bool      `json:"passed"`
	Feedback  string    `json:"feedback"`
	CreatedAt time.Time `json:"created_at"`
}

// Store aggregates all data access.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Pool returns the underlying connection pool.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// GetOrCreateUser finds a user by Google sub, or inserts a new one with the free-tier allowance.
func (s *Store) GetOrCreateUser(ctx context.Context, sub, email, name, picture string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (google_sub, email, name, picture, role, credits)
		VALUES ($1, $2, $3, $4, $5, 10)
		ON CONFLICT (google_sub) DO UPDATE
			SET email = EXCLUDED.email,
			    name = EXCLUDED.name,
			    picture = EXCLUDED.picture,
			    updated_at = now()
		RETURNING id, google_sub, email, name, picture, role, credits, created_at, updated_at
	`, sub, email, name, picture).Scan(
		&u.ID, &u.GoogleSub, &u.Email, &u.Name, &u.Picture, &u.Role, &u.Credits, &u.CreatedAt, &time.Time{},
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByID returns a user by primary key.
func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	var u User
	var updatedAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, google_sub, email, name, picture, role, credits, created_at, updated_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.GoogleSub, &u.Email, &u.Name, &u.Picture, &u.Role, &u.Credits, &u.CreatedAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserBySub returns a user by Google subject identifier.
func (s *Store) GetUserBySub(ctx context.Context, sub string) (*User, error) {
	var u User
	var updatedAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, google_sub, email, name, picture, role, credits, created_at, updated_at
		FROM users WHERE google_sub = $1
	`, sub).Scan(&u.ID, &u.GoogleSub, &u.Email, &u.Name, &u.Picture, &u.Role, &u.Credits, &u.CreatedAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// SetUserRole updates a user's role (admin tooling).
func (s *Store) SetUserRole(ctx context.Context, id uuid.UUID, role string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET role = $1, updated_at = now() WHERE id = $2`, role, id)
	return err
}

// AdjustCredits atomically adds delta to a user's credit balance and records the event.
// Returns the new balance or an error if the balance would go below zero.
func (s *Store) AdjustCredits(ctx context.Context, tx pgx.Tx, userID uuid.UUID, delta int, reason, attemptID string) (int, error) {
	var newBalance int
	aid := sql.NullString{String: attemptID, Valid: attemptID != ""}
	err := tx.QueryRow(ctx, `
		UPDATE users
		SET credits = credits + $1, updated_at = now()
		WHERE id = $2 AND (credits + $1) >= 0
		RETURNING credits
	`, delta, userID).Scan(&newBalance)
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO credit_events (user_id, attempt_id, delta, reason)
		VALUES ($1, $2, $3, $4)
	`, userID, aid, delta, reason)
	return newBalance, err
}

// ListChallenges returns published challenges, optionally filtered by category and difficulty.
func (s *Store) ListChallenges(ctx context.Context, category, difficulty string) ([]Challenge, error) {
	query := `SELECT id, title, description, category, difficulty, instructions, hints, points, is_published, created_at FROM challenges WHERE is_published = true`
	var args []any
	argIdx := 1
	addFilter := func(col, val string) {
		query += ` AND ` + col + ` = $` + itoa(argIdx)
		args = append(args, val)
		argIdx++
	}
	if category != "" {
		addFilter("category", category)
	}
	if difficulty != "" {
		addFilter("difficulty", difficulty)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChallenges(rows)
}

// GetChallenge returns a single challenge by ID (published only unless admin).
func (s *Store) GetChallenge(ctx context.Context, id uuid.UUID, publishedOnly bool) (*Challenge, error) {
	query := `SELECT id, title, description, category, difficulty, instructions, hints, points, is_published, created_at FROM challenges WHERE id = $1`
	args := []any{id}
	if publishedOnly {
		query += ` AND is_published = true`
	}
	var c Challenge
	var hints []byte
	var updatedAt time.Time
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&c.ID, &c.Title, &c.Description, &c.Category, &c.Difficulty, &c.Instructions,
		&hints, &c.Points, &c.IsPublished, &c.VerificationJSON, &c.ExpectedAnswer, &c.CreatedAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = parseJSONHints(hints, &c.Hints)
	return &c, nil
}

// ListAdminChallenges returns all challenges for admin dashboard.
func (s *Store) ListAdminChallenges(ctx context.Context) ([]Challenge, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, title, description, category, difficulty, instructions, hints, points, is_published, created_at FROM challenges ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChallenges(rows)
}

// CreateChallenge inserts a new challenge.
func (s *Store) CreateChallenge(ctx context.Context, c Challenge) (*Challenge, error) {
	var out Challenge
	var hints, verification []byte
	var createdAt, updatedAt time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO challenges (title, description, category, difficulty, instructions, hints, points, is_published, verification, expected_answer)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, title, description, category, difficulty, instructions, hints, points, is_published, verification, expected_answer, created_at, updated_at
	`, c.Title, c.Description, c.Category, c.Difficulty, c.Instructions, marshalHints(c.Hints), c.Points, c.IsPublished, []byte("{}"), c.ExpectedAnswer).Scan(
		&out.ID, &out.Title, &out.Description, &out.Category, &out.Difficulty, &out.Instructions, &hints, &out.Points, &out.IsPublished, &verification, &out.ExpectedAnswer, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	out.CreatedAt = createdAt
	out.VerificationJSON = string(verification)
	_ = parseJSONHints(hints, &out.Hints)
	return &out, nil
}

// UpdateChallenge modifies an existing challenge.
func (s *Store) UpdateChallenge(ctx context.Context, id uuid.UUID, c Challenge) (*Challenge, error) {
	var out Challenge
	var hints, verification []byte
	var createdAt, updatedAt time.Time
	err := s.pool.QueryRow(ctx, `
		UPDATE challenges
		SET title = $2, description = $3, category = $4, difficulty = $5, instructions = $6, hints = $7, points = $8, is_published = $9, verification = $10, expected_answer = $11, updated_at = now()
		WHERE id = $1
		RETURNING id, title, description, category, difficulty, instructions, hints, points, is_published, verification, expected_answer, created_at, updated_at
	`, id, c.Title, c.Description, c.Category, c.Difficulty, c.Instructions, marshalHints(c.Hints), c.Points, c.IsPublished, []byte("{}"), c.ExpectedAnswer).Scan(
		&out.ID, &out.Title, &out.Description, &out.Category, &out.Difficulty, &out.Instructions, &hints, &out.Points, &out.IsPublished, &verification, &out.ExpectedAnswer, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	out.CreatedAt = createdAt
	out.VerificationJSON = string(verification)
	_ = parseJSONHints(hints, &out.Hints)
	return &out, nil
}

// PublishChallenge toggles the published flag.
func (s *Store) PublishChallenge(ctx context.Context, id uuid.UUID, published bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE challenges SET is_published = $1, updated_at = now() WHERE id = $2`, published, id)
	return err
}

// DeleteChallenge removes a challenge and its attempts/submissions (cascade).
func (s *Store) DeleteChallenge(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM challenges WHERE id = $1`, id)
	return err
}

// CreateAttempt inserts a new in-progress attempt.
func (s *Store) CreateAttempt(ctx context.Context, tx pgx.Tx, userID, challengeID uuid.UUID, envID, envType string, creditsConsumed int) (*Attempt, error) {
	var a Attempt
	var endedAt, solvedAt time.Time
	err := tx.QueryRow(ctx, `
		INSERT INTO attempts (user_id, challenge_id, status, environment_id, environment_type, credits_consumed)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, challenge_id, status, environment_id, environment_type, credits_consumed, started_at, ended_at, solved_at, solve_time_seconds
	`, userID, challengeID, AttemptInProgress, envID, envType, creditsConsumed).Scan(
		&a.ID, &a.UserID, &a.ChallengeID, &a.Status, &a.EnvironmentID, &a.EnvironmentType, &a.CreditsConsumed,
		&a.StartedAt, &endedAt, &solvedAt, &a.SolveTimeSeconds,
	)
	if err != nil {
		return nil, err
	}
	a.EndedAt = endedAt
	a.SolvedAt = solvedAt
	return &a, nil
}

// GetActiveAttempt returns the user's in-progress attempt for a challenge, if any.
func (s *Store) GetActiveAttempt(ctx context.Context, userID, challengeID uuid.UUID) (*Attempt, error) {
	var a Attempt
	var endedAt, solvedAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, challenge_id, status, environment_id, environment_type, credits_consumed, started_at, ended_at, solved_at, solve_time_seconds
		FROM attempts WHERE user_id = $1 AND challenge_id = $2 AND status = $3
		ORDER BY started_at DESC LIMIT 1
	`, userID, challengeID, AttemptInProgress).Scan(
		&a.ID, &a.UserID, &a.ChallengeID, &a.Status, &a.EnvironmentID, &a.EnvironmentType, &a.CreditsConsumed,
		&a.StartedAt, &endedAt, &solvedAt, &a.SolveTimeSeconds,
	)
	if err != nil {
		return nil, err
	}
	a.EndedAt = endedAt
	a.SolvedAt = solvedAt
	return &a, nil
}

// CompleteAttempt marks an attempt as passed or failed.
func (s *Store) CompleteAttempt(ctx context.Context, tx pgx.Tx, attemptID uuid.UUID, status string, solvedAt time.Time, solveTimeSeconds int) error {
	_, err := tx.Exec(ctx, `
		UPDATE attempts
		SET status = $2, ended_at = now(), solved_at = $3, solve_time_seconds = $4
		WHERE id = $1 AND status = $5
	`, attemptID, status, solvedAt, solveTimeSeconds, AttemptInProgress)
	return err
}

// ListMyAttempts returns a user's recent attempts.
func (s *Store) ListMyAttempts(ctx context.Context, userID uuid.UUID, limit int) ([]Attempt, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, challenge_id, status, environment_id, environment_type, credits_consumed, started_at, ended_at, solved_at, solve_time_seconds
		FROM attempts WHERE user_id = $1
		ORDER BY started_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAttempts(rows)
}

// CreateSubmission records a solution check.
func (s *Store) CreateSubmission(ctx context.Context, tx pgx.Tx, attemptID uuid.UUID, answer string, passed bool, feedback string) (*Submission, error) {
	var s2 Submission
	var createdAt time.Time
	err := tx.QueryRow(ctx, `
		INSERT INTO submissions (attempt_id, answer, passed, feedback)
		VALUES ($1, $2, $3, $4)
		RETURNING id, attempt_id, answer, passed, feedback, created_at
	`, attemptID, answer, passed, feedback).Scan(
		&s2.ID, &s2.AttemptID, &s2.Answer, &s2.Passed, &s2.Feedback, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	s2.CreatedAt = createdAt
	return &s2, nil
}

// GetChallengeByIDForAdmin fetches a challenge by id for admin (no published filter).
func (s *Store) GetChallengeByIDForAdmin(ctx context.Context, id uuid.UUID) (*Challenge, error) {
	return s.GetChallenge(ctx, id, false)
}

func scanChallenges(rows pgx.Rows) ([]Challenge, error) {
	var out []Challenge
	for rows.Next() {
		var c Challenge
		var hints []byte
		var updatedAt time.Time
		if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.Category, &c.Difficulty, &c.Instructions, &hints, &c.Points, &c.IsPublished, &c.VerificationJSON, &c.ExpectedAnswer, &c.CreatedAt, &updatedAt); err != nil {
			return nil, err
		}
		_ = parseJSONHints(hints, &c.Hints)
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanAttempts(rows pgx.Rows) ([]Attempt, error) {
	var out []Attempt
	for rows.Next() {
		var a Attempt
		var endedAt, solvedAt time.Time
		if err := rows.Scan(&a.ID, &a.UserID, &a.ChallengeID, &a.Status, &a.EnvironmentID, &a.EnvironmentType, &a.CreditsConsumed, &a.StartedAt, &endedAt, &solvedAt, &a.SolveTimeSeconds); err != nil {
			return nil, err
		}
		a.EndedAt = endedAt
		a.SolvedAt = solvedAt
		out = append(out, a)
	}
	return out, rows.Err()
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	// fallback for larger indexes
	buf := make([]byte, 0, 4)
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	return string(buf)
}

func marshalHints(hints []string) []byte {
	if hints == nil {
		return []byte("[]")
	}
	b, _ := json.Marshal(hints)
	return b
}

func parseJSONHints(b []byte, out *[]string) error {
	return json.Unmarshal(b, out)
}
